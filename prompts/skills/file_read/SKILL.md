---
name: file_read
tool_id: file_read
version: "1.0.0"
category: query
requires_approval: false
dry_run: false
invariants:
  - "path must be non-empty"
  - "path traversal (../) is rejected"
  - "control characters in path are rejected"
  - "timeout: 10 seconds"
---

# file_read

Read the contents of a file.

## Parameters

| Name | Type   | Required | Description          |
|------|--------|----------|----------------------|
| path | string | yes      | Absolute file path   |

## Usage

```json
{
  "tool": "file_read",
  "args": {
    "path": "/home/user/document.txt"
  }
}
```

## Response

Returns file contents as a string.

## Safety

- **Path validation**: Traversal (`../`) and control characters rejected
- **Timeout**: 10 seconds
