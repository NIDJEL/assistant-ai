"""Record the installed runtime dependency graph, excluding development-only tools."""

from importlib import metadata
from pathlib import Path

from packaging.requirements import Requirement

root = Path(__file__).resolve().parent.parent
pending = [Requirement(line).name for line in (root / "requirements.txt").read_text().splitlines() if line and not line.startswith("#")]
found = {}
while pending:
    name = pending.pop()
    dist = metadata.distribution(name)
    key = dist.metadata["Name"].lower().replace("_", "-")
    if key in found:
        continue
    found[key] = dist.version
    for dependency in dist.requires or []:
        requirement = Requirement(dependency)
        # Include Windows-only requirements too, so the lock is cross-platform.
        if (
            not requirement.marker
            or requirement.marker.evaluate({"extra": ""})
            or "sys_platform" in str(requirement.marker)
            and "extra" not in str(requirement.marker)
        ):
            pending.append(requirement.name)
(root / "requirements.lock").write_text(
    "# Generated with Python 3.12; runtime dependencies only.\n"
    + "\n".join(f"{name}=={version}" for name, version in sorted(found.items()))
    + "\n",
    encoding="utf-8",
)
print(f"Locked {len(found)} runtime packages")
