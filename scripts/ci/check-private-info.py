#!/usr/bin/env python3
"""Block personal details in tracked files: home folders with a real user
name, personal email addresses, private tailnet names and named Mac hosts.

The patterns are general on purpose. A list of real names here would publish
the very details it is meant to keep out. Use neutral examples instead:
/Users/demo, user@example.com, host.example.ts.net, dev-mac.local.
"""
import re
import subprocess
import sys

PLACEHOLDER_USERS = (
    r"demo|example|user|username|you|me|name|runner|runneradmin|shared|"
    r"public|default|admin|root|ubuntu|pi|nobody|labtether|"
    # Fictional names already used by tests and service accounts.
    r"qa|test|tester|operator|captain|owned|guacd|alice|bob"
)
PATTERNS = {
    "home folder with a real user name": re.compile(
        rf"(?<![\w.])(?:/Users/|/home/)(?!(?i:{PLACEHOLDER_USERS})\b)[A-Za-z][\w.-]*"),
    "Windows home folder with a real user name": re.compile(
        rf"[A-Za-z]:\\\\?Users\\\\?(?!(?:{PLACEHOLDER_USERS})\b)[A-Za-z][\w.-]*",
        re.IGNORECASE),
    "personal email address": re.compile(
        r"[\w.+-]+@(?:gmail|googlemail|icloud|me|mac|hotmail|outlook|live|yahoo|"
        r"proton|protonmail)\.(?:com|me)\b", re.IGNORECASE),
    "private tailnet name": re.compile(r"\btail(?!0example)[0-9a-f]{3,}\.ts\.net\b"),
    "named personal Mac": re.compile(
        r"\b[A-Z][a-z]+s?-(?:MBP|MacBook-(?:Pro|Air)|Mac-mini|iMac)\b"),
}
# Path text that only looks like a home folder.
ALLOWED = re.compile(r"/Users/\$|/home/\$|/Users/<|/home/<|/Users/\{|/home/\{|/Users/\*")


def tracked_text_files():
    out = subprocess.run(["git", "grep", "-I", "-l", "-e", "", "--", "."],
                         capture_output=True, text=True, check=False).stdout
    return out.splitlines()


def main():
    problems = []
    for name in tracked_text_files():
        with open(name, encoding="utf-8", errors="replace") as handle:
            for number, line in enumerate(handle, 1):
                if ALLOWED.search(line):
                    continue
                for label, pattern in PATTERNS.items():
                    match = pattern.search(line)
                    if match:
                        problems.append(f"{name}:{number}: {label}: '{match.group(0)}'")
                        break
    if problems:
        print("Personal details found. Replace them with neutral examples:")
        print("\n".join(f"  {problem}" for problem in problems))
        return 1
    print("No personal details found.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
