#!/usr/bin/env python3
"""Fail if docs/examples claim a pushover registry version that is not published.

Checks README.md, docs/index.md, and examples/provider/main.tf:
  * every pushover `source`/`version` pair must match at least one version
    published on the registry that source resolves to;
  * a short `josh-archer/pushover` source only resolves on OpenTofu by default,
    so any file using it must also document the explicit
    `registry.opentofu.org/josh-archer/pushover` hostname for Terraform users;
  * the stale `~> 1.0` constraint is rejected while only 0.x is published.

Read-only: only GETs the public registry versions APIs. stdlib only.
"""

import json
import re
import sys
import urllib.request

FILES = ["README.md", "docs/index.md", "examples/provider/main.tf"]
OTFU_API = "https://registry.opentofu.org/v1/providers/josh-archer/pushover/versions"
TF_API = "https://registry.terraform.io/v1/providers/josh-archer/pushover/versions"
EXPLICIT_HOST = "registry.opentofu.org/josh-archer/pushover"


def fetch_versions(url):
    req = urllib.request.Request(url, headers={"User-Agent": "docs-honesty-check"})
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return [v["version"] for v in json.load(resp)["versions"]]
    except Exception as exc:  # 404 => provider absent on that registry
        print(f"note: {url} unavailable ({exc}); treating as no versions")
        return []


def parse(version):
    return tuple(int(p) for p in version.split("."))


def satisfies(constraint, version):
    """Support the constraint shapes our docs use: '~> X.Y[.Z]', '= X.Y.Z', 'X.Y.Z'."""
    constraint = constraint.strip()
    if constraint.startswith("~>"):
        base = parse(constraint[2:].strip())
        if len(base) == 2:
            upper = (base[0] + 1, 0, 0)
            lower = (base[0], base[1], 0)
        elif len(base) == 3:
            upper = (base[0], base[1] + 1, 0)
            lower = base
        else:
            raise ValueError(f"unsupported pessimistic constraint: {constraint!r}")
        return lower <= parse(version) < upper
    if constraint.startswith("="):
        return parse(constraint[1:].strip()) == parse(version)
    return parse(constraint) == parse(version)


def main():
    otfu_versions = fetch_versions(OTFU_API)
    tf_versions = fetch_versions(TF_API)
    print(f"OpenTofu registry versions: {otfu_versions or '(none)'}")
    print(f"Terraform registry versions: {tf_versions or '(none)'}")

    failures = []
    pair_re = re.compile(
        r"source\s*=\s*\"([^\"]+)\"\s*\n\s*version\s*=\s*\"([^\"]+)\"", re.MULTILINE
    )
    for path in FILES:
        try:
            text = open(path).read()
        except FileNotFoundError:
            failures.append(f"{path}: file missing")
            continue
        pairs = pair_re.findall(text)
        if "pushover" not in text.lower():
            continue
        documented = [(s, v) for s, v in pairs if "pushover" in s.lower()]
        if not documented:
            failures.append(f"{path}: mentions pushover but has no provider source/version block")
            continue
        for source, version in documented:
            print(f"{path}: source={source!r} version={version!r}")
            try:
                if "." in source.split("/")[0]:
                    host, _ = source.split("/", 1)
                    available = {"registry.opentofu.org": otfu_versions,
                                 "registry.terraform.io": tf_versions}.get(host)
                    if available is None:
                        failures.append(f"{path}: unknown registry host in {source!r}")
                        continue
                    if not any(satisfies(version, v) for v in available):
                        failures.append(
                            f"{path}: {version!r} matches nothing published on {host} "
                            f"(published: {available or '(none)'})"
                        )
                else:
                    # Short source: Terraform -> registry.terraform.io (404 today),
                    # OpenTofu -> registry.opentofu.org.
                    if not any(satisfies(version, v) for v in otfu_versions):
                        failures.append(
                            f"{path}: {version!r} matches nothing on the OpenTofu registry "
                            f"(published: {otfu_versions or '(none)'})"
                        )
                    if EXPLICIT_HOST not in text:
                        failures.append(
                            f"{path}: short source {source!r} does not resolve on "
                            f"registry.terraform.io, but the file never documents the "
                            f"explicit {EXPLICIT_HOST} hostname for Terraform users"
                        )
            except ValueError as exc:
                failures.append(f"{path}: cannot evaluate constraint {version!r} ({exc})")

    if failures:
        print("\nDOCS-HONESTY FAILURES:")
        for failure in failures:
            print(f"  - {failure}")
        sys.exit(1)
    print("\nDocs claim only published registry versions. OK.")


if __name__ == "__main__":
    main()
