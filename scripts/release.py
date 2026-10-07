#!/usr/bin/env python3
"""Validate per-PR versions, extract release notes, and prepare version bumps."""
import argparse
import datetime
import pathlib
import re
import subprocess
from zoneinfo import ZoneInfo

ROOT = pathlib.Path(__file__).resolve().parent.parent
VERSION_PATTERN = r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"


def parse_version(value):
    if not re.fullmatch(VERSION_PATTERN, value):
        raise ValueError("VERSION must be a stable semantic version such as 0.1.0")
    return tuple(map(int, value.split(".")))


def release_notes(changelog, version):
    pattern = rf"^## \[{re.escape(version)}\] - (\d{{4}}-\d{{2}}-\d{{2}})\s*$"
    headings = list(re.finditer(pattern, changelog, re.MULTILINE))
    if len(headings) != 1:
        raise ValueError("CHANGELOG must contain exactly one dated heading for VERSION")
    heading = headings[0]
    datetime.date.fromisoformat(heading.group(1))
    notes = re.split(r"^## ", changelog[heading.end():], maxsplit=1, flags=re.MULTILINE)[0].strip()
    if not notes or re.search(r"\b(TODO|TBD)\b", notes, re.IGNORECASE):
        raise ValueError("Release notes must be nonempty and have no TODO/TBD placeholders")
    return notes


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    check = commands.add_parser("check")
    check.add_argument("--base-ref")
    commands.add_parser("notes")
    bump = commands.add_parser("bump")
    bump.add_argument("kind", choices=["patch", "minor", "major"])
    bump.add_argument("--note", action="append", required=True)
    args = parser.parse_args()
    version_file = ROOT / "VERSION"
    changelog_file = ROOT / "CHANGELOG.md"
    version = version_file.read_text().strip()
    numbers = parse_version(version)
    changelog = changelog_file.read_text()
    if args.command == "bump":
        major, minor, patch = numbers
        next_numbers = {"patch": (major, minor, patch + 1), "minor": (major, minor + 1, 0), "major": (major + 1, 0, 0)}[args.kind]
        next_version = ".".join(map(str, next_numbers))
        if any(not note.strip() or "\n" in note for note in args.note):
            raise ValueError("Each --note must be a nonempty single line")
        date = datetime.datetime.now(ZoneInfo("Europe/Moscow")).date().isoformat()
        entry = f"## [{next_version}] - {date}\n\n" + "\n".join(f"- {note}" for note in args.note) + "\n\n"
        position = changelog.find("\n## ")
        if position == -1:
            raise ValueError("CHANGELOG has no release section")
        next_changelog = changelog[:position + 1] + entry + changelog[position + 1:]
        release_notes(next_changelog, next_version)
        changelog_file.write_text(next_changelog)
        version_file.write_text(next_version + "\n")
        print(next_version)
        return
    notes = release_notes(changelog, version)
    if args.command == "notes":
        print(notes)
        return
    if args.base_ref:
        # First release may introduce VERSION; distinguish absence from a bad ref.
        subprocess.run(["git", "rev-parse", "--verify", args.base_ref], cwd=ROOT, check=True, capture_output=True)
        result = subprocess.run(["git", "show", f"{args.base_ref}:VERSION"], cwd=ROOT, capture_output=True, text=True)
        previous = parse_version(result.stdout.strip()) if result.returncode == 0 else (0, 0, 0)
        if numbers <= previous:
            raise ValueError("Every PR must increase VERSION above the latest base version")
    print(f"Valid release: v{version}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"Release validation failed: {error}")
