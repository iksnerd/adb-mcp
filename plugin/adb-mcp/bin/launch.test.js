'use strict';

// Run with: node --test plugin/adb-mcp/bin/launch.test.js
const test = require('node:test');
const assert = require('node:assert/strict');
const { spawn } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const { resolveBinary, TARGETS } = require('./launch.js');

// Seam 1: which bundled binary serves which machine.
test('resolveBinary maps every shipped target', () => {
  const cases = [
    ['darwin', 'arm64', 'darwin-arm64', 'adb-mcp'],
    ['darwin', 'x64', 'darwin-amd64', 'adb-mcp'],
    ['linux', 'arm64', 'linux-arm64', 'adb-mcp'],
    ['linux', 'x64', 'linux-amd64', 'adb-mcp'],
    ['win32', 'arm64', 'windows-arm64', 'adb-mcp.exe'],
    ['win32', 'x64', 'windows-amd64', 'adb-mcp.exe'],
  ];
  for (const [platform, arch, dir, file] of cases) {
    const got = resolveBinary(platform, arch, '/root/bin');
    assert.equal(got.target, dir);
    assert.equal(got.relPath, `${dir}/${file}`);
    assert.equal(got.path, path.join('/root/bin', dir, file));
  }
  assert.equal(Object.keys(TARGETS).length, cases.length);
});

test('resolveBinary refuses an unsupported machine and names what is supported', () => {
  for (const [platform, arch] of [['freebsd', 'x64'], ['linux', 'ia32'], ['linux', 'riscv64']]) {
    assert.throws(
      () => resolveBinary(platform, arch, '/root/bin'),
      (err) => {
        assert.match(err.message, new RegExp(`${platform}/${arch}`));
        assert.match(err.message, /darwin-arm64/);
        assert.match(err.message, /releases/);
        return true;
      },
    );
  }
});

// Seam 2: the launcher end to end. A copy of launch.js sits in a temp dir next
// to a stub "adb-mcp" for this machine, so nothing in the launcher is faked.
const posix = process.platform !== 'win32';
const sha256 = (file) => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');

function sandbox(script, { checksum = true, tamper = false } = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'launch-'));
  fs.copyFileSync(path.join(__dirname, 'launch.js'), path.join(root, 'launch.js'));
  const { relPath, path: bin } = resolveBinary(process.platform, process.arch, root);
  fs.mkdirSync(path.dirname(bin), { recursive: true });
  fs.writeFileSync(bin, script, { mode: 0o755 });
  if (checksum) fs.writeFileSync(path.join(root, 'checksums.txt'), `${sha256(bin)}  ${relPath}\n`);
  if (tamper) fs.appendFileSync(bin, '# changed after the checksum was taken\n');
  return { root, launcher: path.join(root, 'launch.js'), cleanup: () => fs.rmSync(root, { recursive: true, force: true }) };
}

function run(launcher, args, { input = '', signal } = {}) {
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [launcher, ...args], { stdio: ['pipe', 'pipe', 'pipe'] });
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
    const r = await run(sb.launcher, ['--flag', 'x y'], { input: '{"jsonrpc":"2.0"}\n' });
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
    assert.equal((await run(sb.launcher, [])).code, 7);
  } finally {
    sb.cleanup();
  }
});

test('launcher forwards SIGTERM and exits 128+15', { skip: !posix }, async () => {
  const sb = sandbox('#!/bin/sh\nexec sleep 30\n');
  try {
    const r = await run(sb.launcher, [], { signal: 'SIGTERM' });
    assert.equal(r.code, 143);
  } finally {
    sb.cleanup();
  }
});

test('launcher refuses a binary that differs from checksums.txt and never runs it', { skip: !posix }, async () => {
  const sb = sandbox('#!/bin/sh\necho RAN\n', { tamper: true });
  try {
    const r = await run(sb.launcher, []);
    assert.notEqual(r.code, 0);
    assert.doesNotMatch(r.stdout, /RAN/);
    assert.match(r.stderr, /checksum/i);
  } finally {
    sb.cleanup();
  }
});

test('launcher refuses a binary that checksums.txt does not list', { skip: !posix }, async () => {
  const sb = sandbox('#!/bin/sh\necho RAN\n', { checksum: false });
  try {
    const r = await run(sb.launcher, []);
    assert.notEqual(r.code, 0);
    assert.doesNotMatch(r.stdout, /RAN/);
    assert.match(r.stderr, /checksums\.txt/);
  } finally {
    sb.cleanup();
  }
});
