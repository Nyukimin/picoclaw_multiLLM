# picoclaw_multiLLM Rules

## Source Repository

`picoclaw_multiLLM` is the upstream/source repository for RenCrow Viewer,
runtime, route, adapter, and shared user-facing behavior.

When behavior also exists in `RenCrow_CMD`, apply the shared change here first,
then mirror or sync it to `RenCrow_CMD` only when needed.

## Runtime Routing Rule

Runtime requests from picoclaw must go through the corresponding `RenCrow_XXX`
module. Do not call model backends or tool backends directly from the Viewer or
picoclaw server unless Ren explicitly approves that exception.

Default routing:

- LLM: `picoclaw -> RenCrow_LLM -> LLM backend`
- TTS: `picoclaw -> RenCrow_TTS -> TTS backend`
- STT: `picoclaw -> RenCrow_STT -> STT backend`
- Vision/camera analysis: `picoclaw -> RenCrow_Vision -> Vision or LLM backend`
- Image generation: `picoclaw -> RenCrow_Image -> StableDiffusion / ComfyUI`

Module notes:

- `RenCrow_CMD` is a CLI operation surface, not an independent runtime fork.
- `RenCrow_Code` is empty for now and should not be treated as a runtime target.
- `RenCrow_Image` is the interface module for drawing/image-generation tools such as StableDiffusion and ComfyUI.

## UTF-8 and Path Handling

All source code, docs, config, JSON, and JSONL files should be treated as UTF-8
text unless they are explicitly binary files.

- Treat file names as Unicode paths, not as locale-specific byte strings.
- Do not hard-code Japanese or other non-ASCII paths into PowerShell/cmd command
  strings when filesystem APIs can discover them.
- Prefer Python `pathlib.Path.rglob()` or PowerShell `Get-ChildItem` path
  objects, then operate on those returned paths.
- On Windows, use UTF-8 process I/O for scripts when printing or parsing paths
  (`PYTHONUTF8=1`, `PYTHONIOENCODING=utf-8`, and PowerShell output encoding when
  needed).
- If a file name appears garbled, verify the actual filesystem Unicode name
  before renaming. Only rename when the real name contains replacement
  characters or clear mojibake and the intended UTF-8 name can be determined.
- Git octal-escaped path display is not filename corruption. Use local
  `git config core.quotepath false` for readable non-ASCII paths.
