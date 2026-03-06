---
name: file_list
tool_id: file_list
version: "1.0.0"
category: query
requires_approval: false
dry_run: false
invariants:
  - "path must be non-empty"
  - "path traversal (../) is rejected"
  - "control characters in path are rejected"
  - "output truncated at 1000 entries"
  - "timeout: 10 seconds"
---

# file_list

List files and directories in a given path.

## Parameters

| Name | Type   | Required | Description            |
|------|--------|----------|------------------------|
| path | string | yes      | Directory path to list |

## Usage

```json
{
  "tool": "file_list",
  "args": {
    "path": "/home/user/projects"
  }
}
```

## Response

Returns one entry per line. Directories have a trailing `/`.

```
file1.txt
file2.go
subdir/
```

## Safety

- **Path validation**: Traversal (`../`) and control characters rejected
- **Truncation**: Max 1000 entries
- **Timeout**: 10 seconds
