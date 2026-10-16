"""Write release notes for the version in VERSION.md."""

import pathlib
import re
import sys

sys.path.insert(0, str(pathlib.Path(__file__).parent))
from should_release import parse_version

version = parse_version(pathlib.Path("VERSION.md").read_text(encoding="utf-8"))
changelog = pathlib.Path("CHANGELOG.md").read_text(encoding="utf-8")
match = re.search(rf"## \[{re.escape(version)}\].*?(?=\n## |\Z)", changelog, re.S)
notes = match.group(0).strip() if match else f"Harbor {version}"
pathlib.Path("release-notes.md").write_text(notes + "\n", encoding="utf-8")
