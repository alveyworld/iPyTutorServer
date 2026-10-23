#!/usr/bin/env python3
"""Find a domain's HTTPS server block in nginx configs and include a snippet in it.

Used by add-tutor-path.sh:

    nginx_include.py find <domain> <include line> <config files...>
        prints "<file>\t<yes|no>" for each HTTPS server block whose server_name lists the
        domain; yes when the include line is already in that block
    nginx_include.py edit <domain> <include line> <config file>
        adds the include line after that block's server_name line, in place

A server block counts as HTTPS when it has a "listen ... 443" directive, as certbot writes.
"""
import re
import sys


def server_blocks(text):
    """Yields (start, end) offsets of each top-level "server { ... }" block."""
    for match in re.finditer(r"(?m)^[ \t]*server\s*\{", text):
        depth = 0
        for i in range(match.end() - 1, len(text)):
            if text[i] == "{":
                depth += 1
            elif text[i] == "}":
                depth -= 1
                if depth == 0:
                    yield match.start(), i
                    break


def strip_comments(text):
    return re.sub(r"#[^\n]*", "", text)


def matching_blocks(text, domain):
    """Yields (start, end) of HTTPS server blocks whose server_name lists domain exactly."""
    name = re.compile(r"(?m)^\s*server_name\s+[^;]*(?<![\w.-])" + re.escape(domain) + r"(?![\w.-])")
    for start, end in server_blocks(text):
        block = strip_comments(text[start:end])
        if re.search(r"(?m)^\s*listen\s+[^;]*\b443\b", block) and name.search(block):
            yield start, end


def find(domain, include, files):
    for path in files:
        try:
            text = open(path, encoding="utf-8").read()
        except OSError:
            continue
        for start, end in matching_blocks(text, domain):
            already = include in text[start:end]
            print(f"{path}\t{'yes' if already else 'no'}")


def edit(domain, include, path):
    text = open(path, encoding="utf-8").read()
    blocks = list(matching_blocks(text, domain))
    if len(blocks) != 1:
        sys.exit(f"expected one HTTPS server block for {domain} in {path}, found {len(blocks)}")
    start, end = blocks[0]
    line = re.search(r"(?m)^([ \t]*)server_name\b[^;]*;[^\n]*\n", text[start:end])
    if not line:
        sys.exit(f"no server_name line in the {domain} block of {path}")
    at = start + line.end()
    indent = line.group(1) or "    "
    text = text[:at] + f"\n{indent}# iPyTutor, added by add-tutor-path.sh\n{indent}{include}\n" + text[at:]
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)


if __name__ == "__main__":
    if len(sys.argv) < 5 or sys.argv[1] not in ("find", "edit"):
        sys.exit(__doc__)
    mode, domain, include, *files = sys.argv[1:]
    if mode == "find":
        find(domain, include, files)
    else:
        edit(domain, include, files[0])
