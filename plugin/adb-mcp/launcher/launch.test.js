'use strict';

// Run with: node --test plugin/adb-mcp/launcher/launch.test.js
const test = require('node:test');
const assert = require('node:assert/strict');
const { spawn } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const zlib = require('node:zlib');

const { resolveTarget, ensureBinary, TARGETS } = require('./launch.js');

const sha256 = (buf) => crypto.createHash('sha256').update(buf).digest('hex');
const tmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'launch-'));
const posix = process.platform !== 'win32';

// Seam 1: which release asset serves which machine.
test('resolveTarget maps every shipped target to its asset', () => {
  const cases = [
    ['darwin', 'arm64', 'darwin-arm64', 'adb-mcp'],
    ['darwin', 'x64', 'darwin-amd64', 'adb-mcp'],
    ['linux', 'arm64', 'linux-arm64', 'adb-mcp'],
    ['linux', 'x64', 'linux-amd64', 'adb-mcp'],
    ['win32', 'arm64', 'windows-arm64', 'adb-mcp.exe'],
    ['win32', 'x64', 'windows-amd64', 'adb-mcp.exe'],
  ];
  for (const [platform, arch, target, file] of cases) {
    const got = resolveTarget(platform, arch, '1.2.3');
    assert.equal(got.target, target);
    assert.equal(got.file, file);
    assert.equal(
      got.url,
      `https://github.com/iksnerd/adb-mcp/releases/download/v1.2.3/adb-mcp_v1.2.3_${target.replace('-', '_')}${file === 'adb-mcp.exe' ? '.exe' : ''}.gz`,
    );
  }
  assert.equal(TARGETS.length, cases.length);
});

test('resolveTarget refuses an unsupported machine and names what is supported', () => {
  for (const [platform, arch] of [['freebsd', 'x64'], ['linux', 'ia32'], ['linux', 'riscv64']]) {
    assert.throws(
      () => resolveTarget(platform, arch, '1.2.3'),
      (err) => {
        assert.match(err.message, new RegExp(`${platform}/${arch}`));
        assert.match(err.message, /darwin-arm64/);
        assert.match(err.message, /releases/);
        return true;
      },
    );
  }
});

// Seam 2a: fetching, verifying and caching the server binary.
function fixture({ content = '#!/bin/sh\necho hi\n' } = {}) {
  const binary = Buffer.from(content);
  const target = resolveTarget(process.platform, process.arch, '1.2.3');
  const pin = { version: '1.2.3', assets: { [target.target]: sha256(binary) } };
  const calls = [];
  const fetchBytes = async (url) => {
    calls.push(url);
    return zlib.gzipSync(binary);
  };
  return { binary, target, pin, calls, fetchBytes, dataDir: tmp() };
}

test('ensureBinary downloads once, verifies, and runs from the cache after that', { skip: !posix }, async () => {
  const f = fixture();
  const first = await ensureBinary({ ...f });
  assert.deepEqual(f.calls, [f.target.url]);
  assert.deepEqual(fs.readFileSync(first), f.binary);
  assert.ok(fs.statSync(first).mode & 0o100, 'binary is executable');
  assert.ok(first.startsWith(f.dataDir), 'cache lives under the data dir');

  const second = await ensureBinary({ ...f });
  assert.equal(second, first);
  assert.equal(f.calls.length, 1, 'a verified cache hit does not fetch again');
});

test('ensureBinary replaces a cached binary that no longer matches the pin', { skip: !posix }, async () => {
  const f = fixture();
  const bin = await ensureBinary({ ...f });
  fs.writeFileSync(bin, '#!/bin/sh\necho swapped\n');
  await ensureBinary({ ...f });
  assert.equal(f.calls.length, 2);
  assert.deepEqual(fs.readFileSync(bin), f.binary);
});

test('ensureBinary refuses a download that does not match the pin and caches nothing', async () => {
  const f = fixture();
  const evil = zlib.gzipSync(Buffer.from('not the pinned binary'));
  await assert.rejects(
    ensureBinary({ ...f, fetchBytes: async () => evil }),
    /checksum mismatch/i,
  );
  const cached = [];
  (function walk(dir) {
    for (const e of fs.existsSync(dir) ? fs.readdirSync(dir, { withFileTypes: true }) : []) {
      e.isDirectory() ? walk(path.join(dir, e.name)) : cached.push(e.name);
    }
  })(f.dataDir);
  assert.deepEqual(cached, []);
});

test('ensureBinary refuses a version that has no pin for this target', async () => {
  const f = fixture();
  await assert.rejects(ensureBinary({ ...f, pin: { version: '1.2.3', assets: {} } }), /no pinned checksum/i);
});

test('ensureBinary names the URL when the download fails', async () => {
  const f = fixture();
  await assert.rejects(
    ensureBinary({ ...f, fetchBytes: async () => { throw new Error('getaddrinfo ENOTFOUND github.com'); } }),
    (err) => {
      assert.match(err.message, /ENOTFOUND/);
      assert.match(err.message, /releases\/download\/v1\.2\.3/);
      return true;
    },
  );
});

test('ensureBinary refuses a download that is not gzip', async () => {
  const f = fixture();
  await assert.rejects(ensureBinary({ ...f, fetchBytes: async () => Buffer.from('<html>404</html>') }), /gzip|decompress/i);
});

// Seam 2b: the launcher end to end. A copy of launch.js sits next to a
// release.json pin, with a stub server already in the cache, so the process
// that starts is the real launcher and nothing is faked.
function sandbox(script) {
  const root = tmp();
  const dataDir = tmp();
  fs.copyFileSync(path.join(__dirname, 'launch.js'), path.join(root, 'launch.js'));
  const binary = Buffer.from(script);
  const t = resolveTarget(process.platform, process.arch, '9.9.9');
  fs.writeFileSync(path.join(root, 'release.json'), JSON.stringify({ version: '9.9.9', assets: { [t.target]: sha256(binary) } }));
  const cached = path.join(dataDir, 'adb-mcp', '9.9.9', t.target, t.file);
  fs.mkdirSync(path.dirname(cached), { recursive: true });
  fs.writeFileSync(cached, binary, { mode: 0o755 });
  return { launcher: path.join(root, 'launch.js'), dataDir, cleanup: () => { fs.rmSync(root, { recursive: true, force: true }); fs.rmSync(dataDir, { recursive: true, force: true }); } };
}

function run(sb, args, { input = '', signal, env = {} } = {}) {
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [sb.launcher, ...args], {
      stdio: ['pipe', 'pipe', 'pipe'],
      env: { ...process.env, CLAUDE_PLUGIN_DATA: sb.dataDir, ...env },
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (d) => (stdout += d));
    child.stderr.on('data', (d) => (stderr += d));
    child.on('close', (code, sig) => resolve({ code, sig, stdout, stderr }));
    if (signal) setTimeout(() => child.kill(signal), 400);
    child.stdin.end(input);
  });
}

test('launcher passes arguments and stdio straight through', { skip: !posix }, async () => {
  const sb = sandbox('#!/bin/sh\necho "args:$*"\ncat\n');
  try {
    const r = await run(sb, ['--flag', 'x y'], { input: '{"jsonrpc":"2.0"}\n' });
    assert.equal(r.code, 0);
    assert.equal(r.stdout, 'args:--flag x y\n{"jsonrpc":"2.0"}\n');
    assert.equal(r.stderr, '');
  } finally {
    sb.cleanup();
  }
});

test('launcher exits with the server exit code', { skip: !posix }, async () => {
  const sb = sandbox('#!/bin/sh\nexit 7\n');
  try {
    assert.equal((await run(sb, [])).code, 7);
  } finally {
    sb.cleanup();
  }
});

test('launcher forwards SIGTERM and exits 128+15', { skip: !posix }, async () => {
  const sb = sandbox('#!/bin/sh\nexec sleep 30\n');
  try {
    assert.equal((await run(sb, [], { signal: 'SIGTERM' })).code, 143);
  } finally {
    sb.cleanup();
  }
});

test('launcher says what is missing when CLAUDE_PLUGIN_DATA is not set', { skip: !posix }, async () => {
  const sb = sandbox('#!/bin/sh\necho RAN\n');
  try {
    const r = await run(sb, [], { env: { CLAUDE_PLUGIN_DATA: '' } });
    assert.notEqual(r.code, 0);
    assert.doesNotMatch(r.stdout, /RAN/);
    assert.match(r.stderr, /CLAUDE_PLUGIN_DATA/);
  } finally {
    sb.cleanup();
  }
});
