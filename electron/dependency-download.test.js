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

test('electron-builder filters and packages files with the patched glob dependencies', async (t) => {
  const { FileMatcher } = require('app-builder-lib/out/fileMatcher');
  const { AsarPackager } = require('app-builder-lib/out/asar/asarUtil');
  const asar = await import('@electron/asar');
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'electron-package-test-'));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  const source = path.join(directory, 'app');
  const destination = path.join(directory, 'resources');
  await fs.mkdir(destination);
  const fixture = {
    'package.json': '{"name":"packaging-fixture","main":"main.js"}',
    'main.js': 'module.exports = 42;\n',
    '.config': 'included dotfile',
    'nested/module.js': 'module.exports = true;\n',
    'nested/native.node': 'unpacked native module fixture',
    'nested/module.js.map': 'excluded source map',
    'ignored/private.js': 'excluded directory',
  };
  const metadata = new Map();
  for (const [relative, content] of Object.entries(fixture)) {
    const file = path.join(source, relative);
    await fs.mkdir(path.dirname(file), { recursive: true });
    await fs.writeFile(file, content);
    metadata.set(file, await fs.lstat(file));
  }
  const filter = new FileMatcher(source, destination, (value) => value,
    ['**/*', '!**/*.map', '!ignored{,/**/*}']).createFilter();
  const files = [...metadata.keys()].filter((file) => filter(file, metadata.get(file)));
  const included = ['.config', 'main.js', 'nested/module.js', 'nested/native.node', 'package.json'];
  assert.deepEqual(files.map((file) => path.relative(source, file).split(path.sep).join('/')).sort(), included);
  const unpackPattern = new FileMatcher(source, destination, (value) => value, ['**/*.node']).createFilter();
  const packager = new AsarPackager({ info: { getWorkspaceRoot: async () => directory } }, {
    resourcePath: destination,
    defaultDestination: destination,
    options: { smartUnpack: false },
    unpackPattern,
  });
  await packager.pack([{ src: source, destination, files, metadata }]);
  const archive = path.join(destination, 'app.asar');
  for (const relative of included) {
    assert.equal(asar.extractFile(archive, relative).toString(), fixture[relative]);
  }
  assert.throws(() => asar.statFile(archive, 'nested/module.js.map'));
  assert.throws(() => asar.statFile(archive, 'ignored/private.js'));
  assert.equal(asar.statFile(archive, 'nested/native.node').unpacked, true);
  assert.equal(await fs.readFile(`${archive}.unpacked/nested/native.node`, 'utf8'), fixture['nested/native.node']);

  // Exercise asar's own glob and minimatch calls as well as builder's stream API.
  const globArchive = path.join(destination, 'glob.asar');
  await asar.createPackageWithOptions(source, globArchive, {
    globOptions: { ignore: [`${source.split(path.sep).join('/')}/**/*.map`, `${source.split(path.sep).join('/')}/ignored/**`] },
    unpack: '*.node',
  });
  assert.equal(asar.extractFile(globArchive, 'nested/module.js').toString(), fixture['nested/module.js']);
  assert.equal(asar.statFile(globArchive, 'nested/native.node').unpacked, true);
  assert.throws(() => asar.statFile(globArchive, 'ignored/private.js'));
});

test('filelist retains globstar, brace and exclusion matching', async (t) => {
  const { FileList } = require('filelist');
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'electron-glob-test-'));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  for (const relative of ['main.js', '.hidden.js', 'nested/a.js', 'nested/b.json', 'nested/c.map', 'ignored/private.js']) {
    const file = path.join(directory, relative);
    await fs.mkdir(path.dirname(file), { recursive: true });
    await fs.writeFile(file, 'fixture');
  }
  const list = new FileList();
  // filelist accepts absolute paths without changing the process cwd.
  const prefix = directory.split(path.sep).join('/');
  list.include(`${prefix}/**/*.{js,json}`);
  list.exclude(`${prefix}/ignored/**`);
  assert.deepEqual(list.toArray().map((file) => path.relative(directory, file).split(path.sep).join('/')).sort(),
    ['main.js', 'nested/a.js', 'nested/b.json']);
});
