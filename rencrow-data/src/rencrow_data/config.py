from __future__ import annotations

import json
from pathlib import Path
from typing import Any


def load_config(path: str | Path, default: Any = None) -> Any:
    p = Path(path)
    if not p.exists():
        if default is not None:
            return default
        raise FileNotFoundError(p)
    text = p.read_text(encoding="utf-8").strip()
    if not text:
        return default if default is not None else {}
    return json.loads(text)


def config_path(config_root: str | Path, name: str) -> Path:
    return Path(config_root) / name

