class Lazycap < Formula
  desc "Terminal UI dashboard for Capacitor mobile app development"
  homepage "https://github.com/icarus-itcs/lazycap"
  url "https://github.com/icarus-itcs/lazycap/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER"
  license "MIT"
  head "https://github.com/icarus-itcs/lazycap.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = %W[
      -s -w
      -X main.version=#{version}
      -X main.commit=#{tap.user}
      -X main.date=#{time.iso8601}
    ]
    system "go", "build", *std_go_args(ldflags:), "."
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/lazycap --version")
  end
end
