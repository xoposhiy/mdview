#!/usr/bin/env python3
"""Generate the large test files used by the benchmark scripts.

The small fixtures in testdata/ are checked in; these two are not, because
they are big and fully generated. Run this from the repository root:

    python scripts/make-testdata.py

Needs Pillow for the image.
"""

import os
import random

OUT = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "testdata")


def big_markdown(path):
    """A document large enough for a render to be measurable."""
    random.seed(1)
    words = "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu".split()
    parts = ["# Big benchmark document\n"]
    for section in range(60):
        parts.append("## Section %d\n" % section)
        for _ in range(6):
            parts.append(" ".join(random.choice(words) for _ in range(60)) + "\n")
        parts.append("- item one\n- item two\n- item three\n")
        parts.append("```go\nfunc f%d() int { return %d }\n```\n" % (section, section))
        parts.append("See [[note-%d]] and [[note-%d|alias]] and [link](other.md).\n"
                     % (section, section + 1))
        parts.append("| a | b |\n|---|---|\n| 1 | 2 |\n")
        parts.append("Math: $x^2 + y^2 = z^2$\n")
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(parts))


def big_jpeg(path):
    """A 12-megapixel opaque image, for decode, pack and paint timings."""
    from PIL import Image

    img = Image.new("RGB", (4000, 3000))
    px = img.load()
    for y in range(0, 3000, 4):
        for x in range(0, 4000, 4):
            color = ((x * 7) % 256, (y * 5) % 256, ((x + y) * 3) % 256)
            for dy in range(4):
                for dx in range(4):
                    px[x + dx, y + dy] = color
    img.save(path, quality=85)


def main():
    os.makedirs(OUT, exist_ok=True)
    for name, build in (("bench-big.md", big_markdown), ("big-4000x3000.jpg", big_jpeg)):
        path = os.path.join(OUT, name)
        build(path)
        print("%s  %d bytes" % (name, os.path.getsize(path)))


if __name__ == "__main__":
    main()
