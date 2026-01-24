# Homebrew formula for lazycap
# This file is automatically updated by the release workflow
# Manual changes to url/sha256 will be overwritten

class Lazycap < Formula
  desc "Terminal UI dashboard for Capacitor mobile app development"
  homepage "https://github.com/icarus-itcs/lazycap"
  url "https://github.com/icarus-itcs/lazycap/archive/refs/tags/v0.4.0.tar.gz"
  sha256 "9b207c2204c0a98338ea13a2fe8404772fadcd8607114ae4b7d1fc6d6c23f9d7"
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
    # The TUI requires a terminal, so just verify the binary runs
    assert_match version.to_s, shell_output("#{bin}/lazycap version 2>&1", 0)
  end
end
