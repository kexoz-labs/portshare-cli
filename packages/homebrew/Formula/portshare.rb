class Portshare < Formula
  desc "Instant localhost tunnels for developers — ngrok alternative"
  homepage "https://portshare.kexoz.dev"
  version "1.0.6"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/jagadesh31/Portshare/releases/download/v1.0.6/portshare-darwin-arm64"
      sha256 "PLACEHOLDER"
    end
    on_intel do
      url "https://github.com/jagadesh31/Portshare/releases/download/v1.0.6/portshare-darwin-amd64"
      sha256 "PLACEHOLDER"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/jagadesh31/Portshare/releases/download/v1.0.6/portshare-linux-arm64"
      sha256 "PLACEHOLDER"
    end
    on_intel do
      url "https://github.com/jagadesh31/Portshare/releases/download/v1.0.6/portshare-linux-amd64"
      sha256 "PLACEHOLDER"
    end
  end

  def install
    if OS.mac?
      arch = Hardware::CPU.arm? ? "arm64" : "amd64"
      bin.install "portshare-darwin-#{arch}" => "portshare"
    else
      arch = Hardware::CPU.arm? ? "arm64" : "amd64"
      bin.install "portshare-linux-#{arch}" => "portshare"
    end
  end

  test do
    system "#{bin}/portshare", "--version"
  end
end
