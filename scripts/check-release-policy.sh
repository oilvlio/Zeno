#!/usr/bin/env bash
set -euo pipefail

tag="${1:-${GITHUB_REF_NAME:-}}"
commit="${2:-${GITHUB_SHA:-HEAD}}"
main_ref="${3:-origin/main}"

[[ -n "$tag" ]] || { echo "release tag is required" >&2; exit 1; }
semver_re='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
[[ "$tag" =~ $semver_re ]] || { echo "release tag is not strict SemVer: $tag" >&2; exit 1; }

version=$(tr -d '\r\n' < VERSION)
[[ "$version" == "$tag" ]] || { echo "VERSION ($version) does not match tag ($tag)" >&2; exit 1; }

tag_commit=$(git rev-list -n 1 "refs/tags/$tag" 2>/dev/null || true)
[[ -n "$tag_commit" ]] || { echo "tag does not exist in checkout: $tag" >&2; exit 1; }
commit=$(git rev-parse "$commit^{commit}")
[[ "$tag_commit" == "$commit" ]] || { echo "tag $tag does not point to workflow commit $commit" >&2; exit 1; }
git merge-base --is-ancestor "$commit" "$main_ref" || {
  echo "release commit $commit is not reachable from $main_ref" >&2
  exit 1
}
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "release checkout is not clean" >&2
  exit 1
fi

python3 - "$tag" <<'PY'
import re
import subprocess
import sys

pattern = re.compile(
    r"^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
    r"(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
    r"(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$"
)

def parse(value):
    match = pattern.fullmatch(value)
    if not match:
        return None
    major, minor, patch = map(int, match.group(1, 2, 3))
    prerelease = match.group(4)
    identifiers = None if prerelease is None else prerelease.split(".")
    return major, minor, patch, identifiers

def compare(left, right):
    if left[:3] != right[:3]:
        return (left[:3] > right[:3]) - (left[:3] < right[:3])
    a, b = left[3], right[3]
    if a is None or b is None:
        return (a is None) - (b is None)
    for x, y in zip(a, b):
        if x == y:
            continue
        xn, yn = x.isdigit(), y.isdigit()
        if xn and yn:
            return (int(x) > int(y)) - (int(x) < int(y))
        if xn != yn:
            return -1 if xn else 1
        return (x > y) - (x < y)
    return (len(a) > len(b)) - (len(a) < len(b))

current_text = sys.argv[1]
current = parse(current_text)
tags = subprocess.check_output(["git", "tag", "--list", "v*"], text=True).splitlines()
prior = [(tag, parse(tag)) for tag in tags if tag != current_text]
prior = [(tag, parsed) for tag, parsed in prior if parsed is not None]
if current_text == "v1.0.0":
    # The owner may intentionally rebuild the bootstrap release after a
    # mistaken major-version publication. Historical v1.x aliases must not
    # block that explicitly requested reset.
    prior = []
elif current[0] == 0 and current[1] >= 10:
    # v1.0.0-v1.9.x are protected aliases from the pre-renumber release line.
    # They cannot be deleted under the repository ruleset and must not outrank
    # their canonical v0.10.0-v0.19.x replacements.
    prior = [(tag, parsed) for tag, parsed in prior if not (parsed[0] == 1 and parsed[1] <= 9)]
not_lower = [tag for tag, parsed in prior if compare(parsed, current) >= 0]
if not_lower:
    print(
        f"release {current_text} is not greater than existing SemVer tag(s): {', '.join(sorted(not_lower))}",
        file=sys.stderr,
    )
    raise SystemExit(1)
PY
