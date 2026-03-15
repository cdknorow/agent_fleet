"""Generic utilities and configuration for Coral."""

from __future__ import annotations

import asyncio
import os
import subprocess
from pathlib import Path
from typing import Tuple

# Configuration Constants
import tempfile
LOG_DIR = tempfile.gettempdir().rstrip("/")
LOG_PATTERN = f"{LOG_DIR}/*_coral_*.log"

# Ensure common macOS binary paths are in PATH so tmux can be found
# when running inside a .app bundle (which has a restricted PATH).
_EXTRA_PATHS = ["/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"]
for _p in _EXTRA_PATHS:
    if _p not in os.environ.get("PATH", "") and os.path.isdir(_p):
        os.environ["PATH"] = _p + ":" + os.environ.get("PATH", "")


def get_package_dir() -> Path:
    """Return the root coral package directory.

    Inside a py2app .app bundle, resources are at $RESOURCEPATH/coral.
    Otherwise, returns the ``src/coral`` directory relative to this file.
    """
    resource_path = os.environ.get("RESOURCEPATH")
    if resource_path:
        return Path(resource_path) / "coral"
    return Path(__file__).resolve().parent.parent  # tools/ -> coral/

HISTORY_PATH = Path(os.environ.get("CLAUDE_PROJECTS_DIR", Path.home() / ".claude" / "projects"))
GEMINI_HISTORY_BASE = Path(os.environ.get("GEMINI_TMP_DIR", Path.home() / ".gemini" / "tmp"))


async def run_cmd(*args: str, timeout: float | None = None) -> Tuple[int, str, str]:
    """Execute a subprocess command asynchronously.

    Returns:
        Tuple of (returncode, stdout, stderr).
    """
    try:
        proc = await asyncio.create_subprocess_exec(
            *args,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        
        if timeout is not None:
            stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=timeout)
        else:
            stdout, stderr = await proc.communicate()
            
        return proc.returncode or 0, stdout.decode().strip(), stderr.decode().strip()
    except asyncio.TimeoutError:
        # If timeout, try to terminate the process
        if proc:
            try:
                proc.terminate()
                await asyncio.wait_for(proc.wait(), timeout=1.0)
            except Exception:
                pass
        return -1, "", "Command timed out"
    except Exception as e:
        return -1, "", str(e)
