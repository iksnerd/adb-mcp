#!/usr/bin/env node
'use strict';

// Starts the adb-mcp server bundled in this plugin. Claude Code runs it as
// `node ${CLAUDE_PLUGIN_ROOT}/bin/launch.js`: it picks the binary built for this
// machine, checks it against checksums.txt, and hands it stdin and stdout
// unchanged, so the MCP stream never passes through Node.

const { spawn } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const OS_NAMES = { darwin: 'darwin', linux: 'linux', win32: 'windows' };
const ARCH_NAMES = { arm64: 'arm64', x64: 'amd64' };
const SIGNAL_NUMBERS = { SIGHUP: 1, SIGINT: 2, SIGTERM: 15 };
const RELEASES_URL = 'https://github.com/iksnerd/adb-mcp/releases/latest';

const TARGETS = Object.fromEntries(
  Object.values(OS_NAMES).flatMap((os) =>
    Object.values(ARCH_NAMES).map((arch) => [`${os}-${arch}`, os === 'windows' ? 'adb-mcp.exe' : 'adb-mcp']),
  ),
);

function resolveBinary(platform, arch, root) {
  const target = `${OS_NAMES[platform]}-${ARCH_NAMES[arch]}`;
  const file = TARGETS[target];
  if (!file) {
    throw new Error(
      `adb-mcp has no bundled build for ${platform}/${arch}. Bundled: ${Object.keys(TARGETS).join(', ')}. ` +
        `Build from source or see ${RELEASES_URL}.`,
    );
  }
  return { target, relPath: `${target}/${file}`, path: path.join(root, target, file) };
}

function expectedChecksum(root, relPath) {
  const listFile = path.join(root, 'checksums.txt');
  let text;
  try {
    text = fs.readFileSync(listFile, 'utf8');
  } catch (err) {
    throw new Error(`cannot read ${listFile}: ${err.message}`);
  }
  for (const line of text.split('\n')) {
    const match = /^([0-9a-f]{64})  (.+)$/.exec(line.trim());
    if (match && match[2] === relPath) return match[1];
  }
  throw new Error(`${relPath} is not listed in ${listFile}`);
}

function verifyChecksum(root, bin) {
  const want = expectedChecksum(root, bin.relPath);
  let data;
  try {
    data = fs.readFileSync(bin.path);
  } catch (err) {
    throw new Error(`cannot read ${bin.path}: ${err.message}`);
  }
  const got = crypto.createHash('sha256').update(data).digest('hex');
  if (got !== want) {
    throw new Error(`checksum mismatch for ${bin.relPath}: checksums.txt says ${want}, the file is ${got}`);
  }
}

function main() {
  let bin;
  try {
    bin = resolveBinary(process.platform, process.arch, __dirname);
    verifyChecksum(__dirname, bin);
  } catch (err) {
    process.stderr.write(`adb-mcp launcher: ${err.message}\n`);
    process.exit(1);
  }

  const child = spawn(bin.path, process.argv.slice(2), { stdio: 'inherit' });
  for (const signal of Object.keys(SIGNAL_NUMBERS)) {
    process.on(signal, () => child.kill(signal));
  }
  child.on('error', (err) => {
    process.stderr.write(`adb-mcp launcher: cannot start ${bin.path}: ${err.message}\n`);
    process.exit(1);
  });
  child.on('exit', (code, signal) => {
    process.exit(signal ? 128 + (SIGNAL_NUMBERS[signal] || 0) : code);
  });
}

if (require.main === module) main();

module.exports = { resolveBinary, TARGETS };
