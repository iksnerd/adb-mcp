#!/usr/bin/env node
'use strict';

// Starts the adb-mcp server for this plugin. Claude Code runs it as
// `node ${CLAUDE_PLUGIN_ROOT}/launcher/launch.js ${CLAUDE_PLUGIN_DATA}`: the
// first argument is the directory to keep the server in, and the rest are passed
// to the server. The launcher reads nothing from the environment.
//
// The server is a Go binary, too large to ship inside a plugin, so the first run
// downloads the one build for this machine from this project's GitHub release,
// checks it against the SHA-256 pinned in release.json, and keeps it under that
// directory. Every start re-checks the cached file before running it.
// The server then gets stdin and stdout unchanged, so the MCP stream never
// passes through Node.

const { spawn } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const zlib = require('node:zlib');

const REPO = 'iksnerd/adb-mcp';
const OS_NAMES = { darwin: 'darwin', linux: 'linux', win32: 'windows' };
const ARCH_NAMES = { arm64: 'arm64', x64: 'amd64' };
const TARGETS = Object.values(OS_NAMES).flatMap((os) => Object.values(ARCH_NAMES).map((arch) => `${os}-${arch}`));
const SIGNAL_NUMBERS = { SIGHUP: 1, SIGINT: 2, SIGTERM: 15 };
const MAX_BINARY_BYTES = 64 << 20;
const DOWNLOAD_TIMEOUT_MS = 120000;

const sha256 = (bytes) => crypto.createHash('sha256').update(bytes).digest('hex');

function resolveTarget(platform, arch, version) {
  const os = OS_NAMES[platform];
  const cpu = ARCH_NAMES[arch];
  if (!os || !cpu) {
    throw new Error(
      `adb-mcp has no build for ${platform}/${arch}. Supported: ${TARGETS.join(', ')}. ` +
        `Build from source or see https://github.com/${REPO}/releases/latest.`,
    );
  }
  const ext = os === 'windows' ? '.exe' : '';
  const tag = `v${version}`;
  return {
    target: `${os}-${cpu}`,
    file: `adb-mcp${ext}`,
    url: `https://github.com/${REPO}/releases/download/${tag}/adb-mcp_${tag}_${os}_${cpu}${ext}.gz`,
  };
}

async function download(url) {
  let res;
  try {
    res = await fetch(url, { redirect: 'follow', signal: AbortSignal.timeout(DOWNLOAD_TIMEOUT_MS) });
  } catch (err) {
    throw new Error(err.cause ? err.cause.message : err.message);
  }
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return Buffer.from(await res.arrayBuffer());
}

function readIfExists(file) {
  try {
    return fs.readFileSync(file);
  } catch (err) {
    if (err.code === 'ENOENT') return null;
    throw err;
  }
}

async function ensureBinary({ pin, target, dataDir, fetchBytes = download }) {
  const want = pin.assets && pin.assets[target.target];
  if (!want) throw new Error(`release.json has no pinned checksum for ${target.target} in v${pin.version}`);

  const dir = path.join(dataDir, 'adb-mcp', pin.version, target.target);
  const dest = path.join(dir, target.file);
  const cached = readIfExists(dest);
  if (cached && sha256(cached) === want) return dest;

  let compressed;
  try {
    compressed = await fetchBytes(target.url);
  } catch (err) {
    throw new Error(`cannot download ${target.url}: ${err.message}`);
  }
  let binary;
  try {
    binary = zlib.gunzipSync(compressed, { maxOutputLength: MAX_BINARY_BYTES });
  } catch (err) {
    throw new Error(`cannot decompress ${target.url}: ${err.message}`);
  }
  const got = sha256(binary);
  if (got !== want) {
    throw new Error(`checksum mismatch for ${target.url}: release.json pins ${want}, the download is ${got}`);
  }

  fs.mkdirSync(dir, { recursive: true });
  const part = `${dest}.${process.pid}.part`;
  fs.writeFileSync(part, binary, { mode: 0o755 });
  fs.renameSync(part, dest);
  return dest;
}

function fail(message) {
  process.stderr.write(`adb-mcp launcher: ${message}\n`);
  process.exit(1);
}

async function main() {
  const [dataDir, ...serverArgs] = process.argv.slice(2);
  let bin;
  try {
    if (!dataDir) throw new Error('no data directory given; the plugin passes ${CLAUDE_PLUGIN_DATA} as the first argument');
    const pin = JSON.parse(fs.readFileSync(path.join(__dirname, 'release.json'), 'utf8'));
    bin = await ensureBinary({ pin, target: resolveTarget(process.platform, process.arch, pin.version), dataDir });
  } catch (err) {
    fail(err.message);
  }

  const child = spawn(bin, serverArgs, { stdio: 'inherit' });
  for (const signal of Object.keys(SIGNAL_NUMBERS)) {
    process.on(signal, () => child.kill(signal));
  }
  child.on('error', (err) => fail(`cannot start ${bin}: ${err.message}`));
  child.on('exit', (code, signal) => {
    process.exit(signal ? 128 + (SIGNAL_NUMBERS[signal] || 0) : code);
  });
}

if (require.main === module) main();

module.exports = { resolveTarget, ensureBinary, TARGETS };
