#!/bin/sh
set -eu

fail() {
	printf 'opera installer: %s\n' "$1" >&2
	exit 1
}

: "${HOME:?HOME must be set}"
repo=${OPERA_REPOSITORY:-diasYuri/opera}
printf '%s\n' "$repo" | grep -Eq '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' || fail "invalid repository: $repo"

os=$(uname -s)
case "$os" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) fail "unsupported operating system: $os (supported: macOS and Linux)" ;;
esac

arch=$(uname -m)
case "$arch" in
	x86_64|x64) arch=amd64 ;;
	aarch64|arm64) arch=arm64 ;;
	*) fail "unsupported architecture: $arch (supported: amd64 and arm64)" ;;
esac

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	fail "sha256sum or shasum is required"
fi

if [ -n "${OPERA_VERSION:-}" ]; then
	tag=$OPERA_VERSION
	case "$tag" in v*) ;; *) tag="v$tag" ;; esac
else
	api="https://api.github.com/repos/$repo/releases?per_page=1"
	releases=$(curl --retry 3 -fsSL -H 'Accept: application/vnd.github+json' "$api") || fail "could not read releases from GitHub"
	tag=$(printf '%s\n' "$releases" | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
fi

version=${tag#v}
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$' || fail "no published semantic-version release found; publish a v*-beta* tag first"

archive="opera_${version}_${os}_${arch}.tar.gz"
root="opera_${version}_${os}_${arch}"
base="https://github.com/$repo/releases/download/$tag"
tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/opera-install.XXXXXX") || fail "could not create a temporary directory"
trap 'rm -rf "$tmpdir"' 0
trap 'exit 1' HUP INT TERM

printf 'Downloading Opera %s for %s/%s...\n' "$version" "$os" "$arch"
curl --retry 3 -fsSL "$base/$archive" -o "$tmpdir/$archive" || fail "could not download $archive"
curl --retry 3 -fsSL "$base/SHA256SUMS" -o "$tmpdir/SHA256SUMS" || fail "could not download SHA256SUMS"

expected=$(awk -v file="$archive" '$2 == file { print $1; exit }' "$tmpdir/SHA256SUMS")
[ "${#expected}" -eq 64 ] || fail "SHA256SUMS has no valid entry for $archive"
case "$expected" in *[!0123456789abcdefABCDEF]*) fail "SHA256SUMS has an invalid digest for $archive" ;; esac
actual=$(sha256 "$tmpdir/$archive")
[ "$actual" = "$expected" ] || fail "checksum verification failed for $archive"

tar -tzf "$tmpdir/$archive" > "$tmpdir/archive.contents" || fail "archive is invalid"
while IFS= read -r member; do
	case "$member" in "$root/"*) ;; *) fail "archive contains an unexpected path: $member" ;; esac
	case "/$member/" in *"/../"*) fail "archive contains a parent path: $member" ;; esac
done < "$tmpdir/archive.contents"
tar -tvzf "$tmpdir/$archive" > "$tmpdir/archive.details" || fail "archive is invalid"
while IFS=' ' read -r mode rest; do
	case "$mode" in -*) ;; *) fail "archive contains a non-regular file" ;; esac
done < "$tmpdir/archive.details"

mkdir -p "$tmpdir/extracted"
tar -xzf "$tmpdir/$archive" -C "$tmpdir/extracted"
binary="$tmpdir/extracted/$root/opera"
[ -f "$binary" ] || fail "package does not contain the Opera executable"

install_dir=${OPERA_INSTALL_DIR:-"$HOME/.local/bin"}
case "$install_dir" in /*) ;; *) fail "OPERA_INSTALL_DIR must be an absolute path" ;; esac
mkdir -p "$install_dir"
staged="$install_dir/.opera-install.$$"
cp "$binary" "$staged"
chmod 0755 "$staged"
mv -f "$staged" "$install_dir/opera"

case ":${PATH:-}:" in
	*":$install_dir:"*) ;;
	*)
		if [ -z "${OPERA_INSTALL_DIR:-}" ]; then
			path_line='export PATH="$HOME/.local/bin:$PATH"'
			case "${SHELL:-}" in
				*/zsh) profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
				*/bash)
					if [ -f "$HOME/.bash_profile" ]; then
						profile="$HOME/.bash_profile"
					else
						profile="$HOME/.bashrc"
					fi
					;;
				*/fish)
					profile="$HOME/.config/fish/config.fish"
					path_line='fish_add_path "$HOME/.local/bin"'
					;;
				*) profile="$HOME/.profile" ;;
			esac
			mkdir -p "$(dirname "$profile")"
			if ! grep -F "$path_line" "$profile" >/dev/null 2>&1; then
				if ! printf '\n# Added by the Opera installer\n%s\n' "$path_line" >> "$profile"; then
					printf 'Could not update %s; add %s to PATH manually.\n' "$profile" "$install_dir" >&2
				fi
			fi
			printf 'PATH configuration is in %s. Open a new shell to use opera.\n' "$profile"
		else
			printf 'Add %s to PATH to run opera directly.\n' "$install_dir"
		fi
		;;
esac

config_dir=${OPERA_CONFIG_DIR:-"$HOME/.opera"}
case "$config_dir" in /*) ;; *) fail "OPERA_CONFIG_DIR must be an absolute path" ;; esac
(umask 077; mkdir -p "$config_dir") || fail "could not create config directory: $config_dir"
if [ ! -e "$config_dir/settings.json" ]; then
	(umask 077; set -C; printf '{}\n' > "$config_dir/settings.json") 2>/dev/null || [ -e "$config_dir/settings.json" ] || fail "could not create settings.json"
fi
if [ ! -e "$config_dir/providers.json" ]; then
	(umask 077; set -C; printf '{\n  "providers": []\n}\n' > "$config_dir/providers.json") 2>/dev/null || [ -e "$config_dir/providers.json" ] || fail "could not create providers.json"
fi
printf 'Configuration initialized at %s\n' "$config_dir"
printf 'Installed Opera %s at %s/opera\n' "$version" "$install_dir"
printf 'Try: opera --help\n'
