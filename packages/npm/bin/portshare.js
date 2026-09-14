#!/usr/bin/env node

const { spawnSync } = require('child_process');
const path = require('path');
const os = require('os');
const fs = require('fs');

const BIN_NAME = os.platform() === 'win32' ? 'portshare.exe' : 'portshare';
const binPath = path.join(__dirname, BIN_NAME);

if (!fs.existsSync(binPath)) {
    console.error('Portshare binary not found! Please run "npm install" again.');
    process.exit(1);
}

const args = process.argv.slice(2);

const result = spawnSync(binPath, args, { stdio: 'inherit' });

if (result.error) {
    console.error('Failed to execute portshare:', result.error);
    process.exit(1);
}

process.exit(result.status !== null ? result.status : 1);
