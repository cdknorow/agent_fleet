"""CLI entry point that executes the bundled launch_agents.sh script."""

import os
import sys


def main():
    from coral.tools.utils import get_package_dir
    script = get_package_dir() / "launch_agents.sh"
    if not script.exists():
        print(f"Error: launch_agents.sh not found at {script}", file=sys.stderr)
        sys.exit(1)
    os.execvp("bash", ["bash", str(script)] + sys.argv[1:])
