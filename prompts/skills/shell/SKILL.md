---
name: shell
tool_id: shell
version: "1.0.0"
category: mutation
requires_approval: true
dry_run: false
invariants:
  - "command must be non-empty string (max 10000 chars)"
  - "control characters are rejected"
  - "timeout: 30 seconds"
---

# shell

Execute a shell command via `sh -c`.

## Parameters

| Name    | Type   | Required | Description            |
|---------|--------|----------|------------------------|
| command | string | yes      | Shell command to run   |

## Usage

```json
{
  "tool": "shell",
  "args": {
    "command": "ls -la /tmp"
  }
}
```

## Response

Returns combined stdout+stderr as a string.

## Safety

- **Requires approval**: Yes (mutation category)
- **Timeout**: 30 seconds
- **Validation**: Non-empty, max 10000 chars, no control characters
