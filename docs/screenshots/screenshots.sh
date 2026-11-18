#!/usr/bin/env bash
# Regenerates the README screenshots with VHS (https://github.com/charmbracelet/vhs).
#
# Needs the dev cluster with generated data: make up && make generate
# Uses vhs from the PATH, or from nixpkgs when nix is available.
set -euo pipefail

cd "$(dirname "$0")/../.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# without the dev tag, so there is no simulated latency
CGO_ENABLED=0 go build -o "$work/ktea" ./cmd/ktea

# an isolated config, the user's ~/.config/ktea is not used
mkdir -p "$work/config/ktea"
install -m 644 docs/screenshots/config/config.yaml "$work/config/ktea/config.yaml"
install -m 600 docs/screenshots/config/connections.yaml "$work/config/ktea/connections.yaml"
export XDG_CONFIG_HOME="$work/config"
export KTEA_BIN="$work/ktea"

if command -v vhs >/dev/null; then
	vhs=(vhs)
elif command -v nix >/dev/null; then
	vhs=(nix shell nixpkgs#vhs --command vhs)
else
	echo "vhs is required, see https://github.com/charmbracelet/vhs#installation" >&2
	exit 1
fi

for tape in topics-page schema-page consumer-lag; do
	echo "Rendering $tape.png"
	"${vhs[@]}" "docs/screenshots/$tape.tape"
done
