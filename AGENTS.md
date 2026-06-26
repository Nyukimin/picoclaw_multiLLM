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
