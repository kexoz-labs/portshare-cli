const fs = require('fs');
const path = require('path');
const os = require('os');
const { execSync } = require('child_process');
const axios = require('axios');

const VERSION = require('./package.json').version;
const BIN_NAME = os.platform() === 'win32' ? 'portshare.exe' : 'portshare';
const BIN_DIR = path.join(__dirname, 'bin');
const BIN_PATH = path.join(BIN_DIR, BIN_NAME);

// Mapping Node's `os.platform()` to Go build OS names
const platformMap = {
    darwin: 'darwin',
    linux: 'linux',
    win32: 'windows'
};

// Mapping Node's `os.arch()` to Go build Arch names
const archMap = {
    x64: 'amd64',
    arm64: 'arm64',
    ia32: '386'
};

async function downloadAndExtract() {
    if (process.env.PORTSHARE_SKIP_DOWNLOAD) {
        console.log('Skipping Portshare binary download because PORTSHARE_SKIP_DOWNLOAD is set.');
        return;
    }

    const platform = platformMap[os.platform()];
    const arch = archMap[os.arch()];

    if (!platform || !arch) {
        console.error(`Unsupported platform or architecture: ${os.platform()}-${os.arch()}`);
        process.exit(1);
    }

    const ext = platform === 'windows' ? '.exe' : '';
    const assetName = `portshare-${platform}-${arch}${ext}`;
    
    // We will use the 'v' prefix for versions to match tags (e.g. v1.0.0)
    const url = `https://github.com/jagadesh31/Portshare/releases/download/v${VERSION}/${assetName}`;
    
    console.log(`Downloading Portshare from: ${url}`);

    try {
        const response = await axios({
            url,
            responseType: 'arraybuffer',
        });

        const archivePath = path.join(BIN_DIR, BIN_NAME);

        if (!fs.existsSync(BIN_DIR)) {
            fs.mkdirSync(BIN_DIR, { recursive: true });
        }

        console.log(`Saving ${assetName}...`);
        fs.writeFileSync(archivePath, response.data);

        // Ensure executable permissions
        if (platform !== 'windows') {
            execSync(`chmod +x "${archivePath}"`);
        }

        console.log('Portshare installed successfully!');

    } catch (error) {
        console.error(`Failed to download or extract the binary: ${error.message}`);
        console.error('Please ensure the version exists in GitHub Releases.');
        process.exit(1);
    }
}

downloadAndExtract();
