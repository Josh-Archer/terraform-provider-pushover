#!/usr/bin/env bash
# Print the next release tag for a commit on main, or nothing when no release
# is needed. Used by .github/workflows/auto-release.yml.
#
#   scripts/next-release-version.sh [commit]   (default HEAD)
#
# - The base is the highest vX.Y.Z tag in the repository. If the commit is
#   already part of it (an ancestor of, or equal to, its commit), nothing is
#   printed: a newer release already covers it.
# - A release is needed when a commit since the base changed something other
#   than docs, Markdown, .github/, examples/ or Go tests. Merge titles are not always
#   conventional, so the decision uses changed files, not prefixes. Commits
#   titled "chore(release): ..." (the version bump the workflow itself pushes)
#   are ignored.
# - Bump: minor for a breaking change ("type!:" subject or "BREAKING CHANGE"
#   in a body) while the major version is 0 (major afterwards), otherwise
#   patch. This keeps the existing v0.x.y scheme.
set -euo pipefail

commit="$(git rev-parse --verify "${1:-HEAD}^{commit}")"

base_tag="$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n 1 || true)"
if [[ -z "${base_tag}" ]]; then
  echo "no vX.Y.Z tag to start from" >&2
  exit 1
fi
base_commit="$(git rev-list -n 1 "${base_tag}")"

if git merge-base --is-ancestor "${commit}" "${base_commit}"; then
  echo "${commit:0:7} is already in ${base_tag}" >&2
  exit 0
fi
if ! git merge-base --is-ancestor "${base_commit}" "${commit}"; then
  echo "${base_tag} is not an ancestor of ${commit:0:7}; refusing to guess a version" >&2
  exit 1
fi

changed="$(git log --no-merges --invert-grep --grep='^chore(release): ' \
    --name-only --format= "${base_commit}..${commit}" \
  | sort -u \
  | grep -vE '^(docs/|examples/|\.github/)|\.md$|_test\.go$' || true)"
if [[ -z "${changed}" ]]; then
  echo "only docs, Markdown, .github/, examples/ or Go tests changed since ${base_tag}" >&2
  exit 0
fi

IFS=. read -r major minor patch <<<"${base_tag#v}"
if git log --format='%s%n%b' "${base_commit}..${commit}" \
  | grep -qE '^[a-z]+(\([^)]*\))?!:|^BREAKING[ -]CHANGE:'; then
  if [[ "${major}" == 0 ]]; then
    minor=$((minor + 1)); patch=0
  else
    major=$((major + 1)); minor=0; patch=0
  fi
else
  patch=$((patch + 1))
fi
echo "v${major}.${minor}.${patch}"
