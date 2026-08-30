#!/usr/bin/env bash
# Build + publish a HorizonX CLI release.
#
# Usage:
#   scripts/release.sh patch              # bump + build, verify, publish
#   scripts/release.sh v0.3.5            # explicit version (same as before)
#   scripts/release.sh patch --dry-run   # build + verify only, no publish
#
# Version semantics (semver 2.0.0, resolved from the latest git tag):
#   major  = breaking change (incompatible agent/server protocol, config
#            format that requires manual migration, dropped feature)
#   minor  = backward-compatible new capability (new endpoints, new pages,
#            new dialogs, redesigns that change behavior)
#   patch  = bug fix or invisible refactor (fixes, wording, styling-only,
#            internal refactors with zero behavior change)
#   Explicit vX.Y.Z skips resolution and publishes exactly that version.
#
# Requires:
#   - docker (pulls golang:1.25-alpine on first run)
#   - gh CLI authenticated for zlnew/horizonx
#
# Contracts (do NOT break these — install.sh + upgrade.go depend on them):
#   1. Tarball names use the installer's arch names: `x86_64`, NOT `amd64`.
#      install.sh normalizes uname -m and requests horizonx-${OS}-${ARCH}.tar.gz.
#   2. The INNER tarball member MUST be named exactly `horizonx` (plain name).
#      install.sh:105 does `tar -xzf ... horizonx`; upgrade.go extractBinary
#      looks for member "horizonx". Arch-qualified members broke v0.3.4's first
#      publish (FATAL: horizonx not found in release tarball).
#   3. Every release ships SHA256SUMS alongside the tarballs.
set -euo pipefail

cd "$(dirname "$0")/.."   # repo root
REPO_ROOT="$PWD"

# The branch releases tag. Default current branch.
RELEASE_BRANCH="${RELEASE_BRANCH:-$(git branch --show-current)}"
echo "== release target branch: $RELEASE_BRANCH =="

RESOLVE="${1:?usage: scripts/release.sh <major|minor|patch|vX.Y.Z> [--dry-run]}"
DRY_RUN="${2:-}"
if [ -n "$DRY_RUN" ] && [ "$DRY_RUN" != "--dry-run" ]; then
  echo "unknown arg: $DRY_RUN (expected --dry-run)" >&2; exit 2
fi

# -- resolve version ----------------------------------------------------------
# Accept a semver bump keyword or an explicit vX.Y.Z. Keywords compute the
# next version from the latest git tag (sorted as versions, not strings).
# Tags are fetched FIRST: a stale local tag list made `patch` resolve below
# an already-published release (v0.4.1 under v0.5.0, caught 2026-08-08).
git fetch --tags origin -q 2>/dev/null || true
case "$RESOLVE" in
  v[0-9]*.[0-9]*.[0-9]*)
    VERSION="$RESOLVE"
    ;;
  major|minor|patch)
    LATEST=$(git tag --sort=-version:refname | head -1 || true)
    LATEST="${LATEST:-v0.0.0}"
    if [[ "$LATEST" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
      MAJOR=${BASH_REMATCH[1]}; MINOR=${BASH_REMATCH[2]}; PATCH=${BASH_REMATCH[3]}
      case "$RESOLVE" in
        major) MAJOR=$((MAJOR+1)); MINOR=0; PATCH=0 ;;
        minor) MINOR=$((MINOR+1)); PATCH=0 ;;
        patch) PATCH=$((PATCH+1)) ;;
      esac
      VERSION="v${MAJOR}.${MINOR}.${PATCH}"
    else
      echo "cannot parse latest tag: $LATEST" >&2; exit 2
    fi
    echo "== version: $RESOLVE bump → $VERSION (latest tag: $LATEST) =="
    ;;
  *)
    echo "usage: scripts/release.sh <major|minor|patch|vX.Y.Z> [--dry-run]" >&2
    exit 2
    ;;
esac
[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "resolved version invalid: $VERSION" >&2; exit 2; }

REPO="zlnew/horizonx"
OUT="/tmp/hx-release-${VERSION}"
TARGETS=("linux x86_64 amd64" "linux arm64 arm64" "darwin x86_64 amd64" "darwin arm64 arm64")
BUILD_IMAGE="golang:1.25-alpine"
BUILD_CONTAINER="hx-build-$$"

# -- preflight ---------------------------------------------------------------
echo "== preflight =="
command -v docker >/dev/null || { echo "docker required" >&2; exit 1; }
command -v gh >/dev/null || { echo "gh CLI required" >&2; exit 1; }

if [ -z "$DRY_RUN" ]; then
  EXISTS=$(gh release view "$VERSION" --repo "$REPO" --json tagName --jq .tagName 2>/dev/null || true)
  [ -z "$EXISTS" ] || { echo "release $VERSION already exists — delete it first if you mean to re-publish" >&2; exit 1; }
fi

# -- build -------------------------------------------------------------------
echo ""
echo "== 1. cross-compile ${#TARGETS[@]} targets =="
rm -rf "$OUT" && mkdir -p "$OUT"

# Spin up a temporary build container, compile all targets, tear down.
echo "  starting build container ($BUILD_IMAGE)…"
docker run -d --name "$BUILD_CONTAINER" \
  -v "$REPO_ROOT:/src" -w /src \
  --entrypoint "" \
  "$BUILD_IMAGE" sleep 3600 >/dev/null

cleanup() { docker rm -f "$BUILD_CONTAINER" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker exec "$BUILD_CONTAINER" sh -c '
  set -e
  apk add --no-cache git >/dev/null 2>&1 || true
  git config --global --add safe.directory /src || true
  mkdir -p /tmp/hx-rel
  for t in '"$(printf '%q ' "${TARGETS[@]}")"'; do
    set -- $t
    os=$1; arch=$2; goarch=$3
    echo "  -- ${os}/${arch}"
    CGO_ENABLED=0 GOOS=$os GOARCH=$goarch \
      go build -trimpath -ldflags "-s -w -X horizonx/internal/version.Version='"$VERSION"'" \
      -o "/tmp/hx-rel/horizonx-${os}-${arch}" ./cmd/horizonx
  done
'

for pair in "${TARGETS[@]}"; do
  set -- $pair
  os=$1; arch=$2
  docker cp "$BUILD_CONTAINER:/tmp/hx-rel/horizonx-${os}-${arch}" "$OUT/"
done

# -- package -----------------------------------------------------------------
echo ""
echo "== 2. package tarballs (member MUST be plain 'horizonx') =="
for pair in "${TARGETS[@]}"; do
  set -- $pair
  os=$1; arch=$2
  # stage a dir so the tarball member is the plain binary name
  STAGE="$OUT/stage-${os}-${arch}"
  rm -rf "$STAGE" && mkdir -p "$STAGE"
  cp "$OUT/horizonx-${os}-${arch}" "$STAGE/horizonx"
  tar -C "$STAGE" -czf "$OUT/horizonx-${os}-${arch}.tar.gz" horizonx
  rm -rf "$STAGE"
  echo "  -> horizonx-${os}-${arch}.tar.gz"
done

# -- verify ------------------------------------------------------------------
echo ""
echo "== 3. verify: member names + checksums + version stamp =="
cd "$OUT"
sha256sum horizonx-*.tar.gz > SHA256SUMS
sha256sum -c SHA256SUMS | tail -4

for pair in "${TARGETS[@]}"; do
  set -- $pair
  os=$1; arch=$2
  MEMBERS=$(tar -tzf "horizonx-${os}-${arch}.tar.gz")
  echo "$MEMBERS" | grep -qx "horizonx" || { echo "FAIL: tarball ${os}-${arch} member is not plain 'horizonx': $MEMBERS" >&2; exit 1; }
  echo "  OK: ${os}-${arch} member=horizonx"
done

for pair in "${TARGETS[@]}"; do
  set -- $pair
  os=$1; arch=$2
  EXDIR=$(mktemp -d)
  tar -xzf "horizonx-${os}-${arch}.tar.gz" -C "$EXDIR" horizonx 2>/dev/null || true
  if [ "$os" = "linux" ]; then
    V=$("$EXDIR/horizonx" version 2>/dev/null | head -1 || true)
    if [ -n "$V" ]; then
      echo "  version(${os}/${arch}): $V"
      [ -z "${V##*"$VERSION"*}" ] || echo "  WARNING: version stamp missing/off for ${os}-${arch}"
    else
      echo "  version(${os}/${arch}): <not runnable on host (cross-arch), stamp trusted from build>"
    fi
  fi
  rm -rf "$EXDIR"
done

echo ""
echo "== artifacts =="
ls -la "$OUT" | grep -E "tar.gz|SHA256"

# -- publish -----------------------------------------------------------------
if [ -n "$DRY_RUN" ]; then
  echo ""
  echo "DRY-RUN: artifacts built + verified, release NOT published."
  exit 0
fi

echo ""
echo "== 4. create GitHub release =="
BODY=$(mktemp)
cat > "$BODY" <<EOF
## $VERSION

$(git -C "$REPO_ROOT" log --oneline "$(git -C "$REPO_ROOT" tag --sort=-version:refname | head -1 2>/dev/null || echo HEAD~10)..HEAD" 2>/dev/null | sed 's/^/- /' | head -40 || true)

4 tarballs + SHA256SUMS. Tarball member is plain \`horizonx\` (install.sh / upgrade.go contract).
EOF

ASSETS=()
for pair in "${TARGETS[@]}"; do
  set -- $pair
  os=$1; arch=$2
  ASSETS+=("$OUT/horizonx-${os}-${arch}.tar.gz")
done
ASSETS+=("$OUT/SHA256SUMS")

gh release create "$VERSION" --repo "$REPO" --title "$VERSION" --notes-file "$BODY" --target "$RELEASE_BRANCH" "${ASSETS[@]}"
rm -f "$BODY"
echo ""
echo "✔ published: https://github.com/$REPO/releases/tag/$VERSION"
