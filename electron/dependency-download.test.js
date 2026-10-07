const assert = require('node:assert/strict');
const { execFile } = require('node:child_process');
const { createHash } = require('node:crypto');
const fs = require('node:fs/promises');
const http = require('node:http');
const https = require('node:https');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');
const { promisify } = require('node:util');

const run = promisify(execFile);
const artifact = 'Electron downloader proxy regression fixture\n';
const checksum = createHash('sha256').update(artifact).digest('hex');
// Public test-only key/certificate, valid 2020–2120 for downloads.test/localhost.
const certificatePath = path.join(__dirname, 'test-fixtures/download-cert.pem');
const privateKeyPath = path.join(__dirname, 'test-fixtures/download-key.pem');

// Each download runs in a fresh process because proxy bootstrap changes Node globals.
// Resolve the downloader used by electron-builder, not Electron's newer installer.
const downloadScript = `
  const assert = require('node:assert/strict');
  const fs = require('node:fs/promises');
  const { createRequire } = require('node:module');
  const builderRequire = createRequire(require.resolve('app-builder-lib/package.json'));
  const { downloadArtifact } = builderRequire('@electron/get');
  const options = JSON.parse(process.env.DOWNLOAD_FIXTURE_OPTIONS);
  downloadArtifact({
    version: '44.4.5',
    artifactName: 'electron',
    platform: 'linux',
    arch: 'x64',
    cacheRoot: options.cacheRoot,
    checksums: { 'electron-v44.4.5-linux-x64.zip': options.checksum },
    mirrorOptions: { resolveAssetURL: () => options.url },
    downloadOptions: {
      quiet: true,
      retry: { limit: 0 },
      timeout: { request: 5000 },
      ...(options.ca ? { https: { certificateAuthority: options.ca } } : {}),
    },
  }).then(async (downloaded) => {
    assert.equal(await fs.readFile(downloaded, 'utf8'), options.artifact);
  }).catch((error) => {
    console.error(JSON.stringify({ code: error.code, message: error.message }));
    process.exitCode = 1;
  });
`;

test('electron-builder downloads retain proxy and TLS behavior with the scoped override', async (t) => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'electron-download-test-'));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  const certificate = await fs.readFile(certificatePath, 'utf8');
  const requests = [];
  const proxyRequests = [];
  const sockets = new Set();
  const serveArtifact = (request, response) => {
    requests.push(request.url);
    response.writeHead(200, { 'content-type': 'application/octet-stream' });
    response.end(artifact);
  };
  const origin = http.createServer(serveArtifact);
  const secureOrigin = https.createServer({
    cert: certificate,
    key: await fs.readFile(privateKeyPath),
  }, serveArtifact);
  const authorization = `Basic ${Buffer.from('fixture:password').toString('base64')}`;
  const proxy = http.createServer((request, response) => {
    proxyRequests.push({ method: request.method, url: request.url });
    if (request.headers['proxy-authorization'] !== authorization) {
      response.writeHead(407);
      response.end();
      return;
    }
    const target = new URL(request.url);
    const upstream = http.get({
      hostname: '127.0.0.1',
      port: origin.address().port,
      path: target.pathname,
      agent: false,
    }, (result) => {
      response.writeHead(result.statusCode, result.headers);
      result.pipe(response);
    });
    upstream.on('error', (error) => response.destroy(error));
  });
  proxy.on('connect', (request, client, head) => {
    proxyRequests.push({ method: request.method, url: request.url });
    if (request.headers['proxy-authorization'] !== authorization) {
      client.end('HTTP/1.1 407 Proxy Authentication Required\r\n\r\n');
      return;
    }
    const upstream = net.connect(secureOrigin.address().port, '127.0.0.1', () => {
      client.write('HTTP/1.1 200 Connection Established\r\n\r\n');
      upstream.write(head);
      client.pipe(upstream);
      upstream.pipe(client);
    });
    sockets.add(upstream);
    upstream.on('close', () => sockets.delete(upstream));
    upstream.on('error', () => client.destroy());
    client.on('error', () => upstream.destroy());
    client.on('close', () => upstream.destroy());
  });
  for (const server of [origin, secureOrigin, proxy]) {
    server.on('connection', (socket) => {
      sockets.add(socket);
      socket.on('close', () => sockets.delete(socket));
    });
    await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
    t.after(() => new Promise((resolve) => {
      for (const socket of sockets) socket.destroy();
      server.close(resolve);
    }));
  }

  const proxyUrl = `http://fixture:password@127.0.0.1:${proxy.address().port}`;
  const baseEnvironment = { ...process.env };
  for (const key of Object.keys(baseEnvironment)) {
    if (/proxy|^electron_|^npm_config_electron_|^npm_package_config_electron_|^node_extra_ca_certs$|^node_tls_reject_unauthorized$|^node_options$/i.test(key)) {
      delete baseEnvironment[key];
    }
  }

  const cases = [
    { name: 'HTTP mirror through an authenticated proxy', scheme: 'http', proxyMethod: 'GET' },
    { name: 'HTTPS CONNECT trusts NODE_EXTRA_CA_CERTS', scheme: 'https', trust: 'environment', proxyMethod: 'CONNECT' },
    { name: 'HTTPS CONNECT trusts a request-specific CA', scheme: 'https', trust: 'request', proxyMethod: 'CONNECT' },
    { name: 'HTTPS CONNECT rejects an untrusted certificate', scheme: 'https', error: /DEPTH_ZERO_SELF_SIGNED_CERT/, proxyMethod: 'CONNECT' },
    { name: 'HTTPS CONNECT rejects a hostname mismatch', scheme: 'https', host: 'wrong-host.test', trust: 'environment', error: /ERR_TLS_CERT_ALTNAME_INVALID/, proxyMethod: 'CONNECT' },
    { name: 'NO_PROXY bypasses the configured proxy', scheme: 'http', host: 'localhost', noProxy: 'localhost' },
  ];
  for (const [index, scenario] of cases.entries()) {
    await t.test(scenario.name, async () => {
      requests.length = 0;
      proxyRequests.length = 0;
      const host = scenario.host || 'downloads.test';
      const port = (scenario.scheme === 'https' ? secureOrigin : origin).address().port;
      const url = `${scenario.scheme}://${host}:${port}/artifact.zip`;
      const env = {
        ...baseEnvironment,
        ELECTRON_GET_USE_PROXY: '1',
        ELECTRON_GET_NO_PROGRESS: '1',
        // Exercise the overridden logger's real implementation, not its disabled stub.
        ROARR_LOG: 'true',
        HTTP_PROXY: proxyUrl,
        HTTPS_PROXY: proxyUrl,
        NO_PROXY: scenario.noProxy || '',
        DOWNLOAD_FIXTURE_OPTIONS: JSON.stringify({
          cacheRoot: path.join(directory, String(index)),
          url,
          artifact,
          checksum,
          ca: scenario.trust === 'request' ? certificate : undefined,
        }),
      };
      if (scenario.trust === 'environment') env.NODE_EXTRA_CA_CERTS = certificatePath;
      const download = run(process.execPath, ['-e', downloadScript], { cwd: __dirname, env, timeout: 10000 });
      if (scenario.error) {
        await assert.rejects(download, scenario.error);
        assert.deepEqual(requests, []);
      } else {
        await download;
        assert.deepEqual(requests, ['/artifact.zip']);
      }
      assert.deepEqual(proxyRequests, scenario.proxyMethod ? [{
        method: scenario.proxyMethod,
        url: scenario.proxyMethod === 'CONNECT' ? `${host}:${port}` : url,
      }] : []);
    });
  }
});
