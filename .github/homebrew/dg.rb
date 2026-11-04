class Dg < Formula
  desc "Diagram-as-code: HCL model, layout arranged in the browser, arrows routed on draw"
  homepage "https://nvcnvn.github.io/dg/"
  version "@VERSION@"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "https://github.com/nvcnvn/dg/releases/download/v@VERSION@/dg-v@VERSION@-darwin-arm64.tar.gz"
      sha256 "@darwin-arm64@"
    end
    on_intel do
      url "https://github.com/nvcnvn/dg/releases/download/v@VERSION@/dg-v@VERSION@-darwin-amd64.tar.gz"
      sha256 "@darwin-amd64@"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/nvcnvn/dg/releases/download/v@VERSION@/dg-v@VERSION@-linux-arm64.tar.gz"
      sha256 "@linux-arm64@"
    end
    on_intel do
      url "https://github.com/nvcnvn/dg/releases/download/v@VERSION@/dg-v@VERSION@-linux-amd64.tar.gz"
      sha256 "@linux-amd64@"
    end
  end

  def install
    bin.install "dg"
    prefix.install "THIRD_PARTY_NOTICES"
  end

  def caveats
    "dg export draws pictures in headless Google Chrome or Chromium, which must be installed."
  end

  test do
    (testpath/"dg.hcl").write "project \"test\" {}\n"
    (testpath/"model.hcl").write "node \"a\" {}\n"
    system bin/"dg", "check", "-C", testpath
  end
end
