const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function desktopFixture(environment) {
  const launches = [];
  const requests = [];
  const child = new EventEmitter();
  child.stdout = new EventEmitter();
  child.stderr = new EventEmitter();

  const modules = {
    electron: {
      app: {
        getPath: () => '/desktop-user-data',
        whenReady: () => ({ then() {} }),
        on() {},
      },
    },
    child_process: {
      spawn: (binary, args, options) => {
        launches.push({ binary, args, options });
        return child;
      },
    },
    path,
    fs: { existsSync: () => true },
    http: {
      get: (options, callback) => {
        requests.push(options);
        const request = new EventEmitter();
        request.destroy = () => {};
        queueMicrotask(() => callback({ statusCode: 200 }));
        return request;
      },
    },
  };
  const context = vm.createContext({
    require: (name) => {
      assert.ok(Object.hasOwn(modules, name), `Unexpected dependency: ${name}`);
      return modules[name];
    },
    process: { env: { ...environment }, resourcesPath: '/desktop-resources', platform: 'win32' },
    __dirname,
    console: { log() {}, error() {} },
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'main.js'), 'utf8'), context);
  return { context, launches, requests };
}

test('production desktop backend overrides inherited public binding with loopback', async () => {
  const { context, launches, requests } = desktopFixture({
    BIND_ADDRESS: '0.0.0.0',
    OTHER_SETTING: 'preserved',
  });
  await context.startServer();

  assert.equal(launches.length, 1);
  assert.equal(launches[0].options.env.BIND_ADDRESS, '127.0.0.1');
  assert.equal(launches[0].options.env.PORT, '3000');
  assert.equal(launches[0].options.env.OTHER_SETTING, 'preserved');
  assert.equal(launches[0].options.env.SQLITE_PATH, path.join('/desktop-user-data', 'data', 'new-api.db'));
  assert.equal(requests[0].hostname, '127.0.0.1');
});

test('development desktop continues using the separately started local frontend', async () => {
  const { context, launches, requests } = desktopFixture({ NODE_ENV: 'development' });
  await context.startServer();

  assert.equal(launches.length, 0);
  assert.equal(requests[0].hostname, '127.0.0.1');
  assert.equal(requests[0].port, 5173);
});
