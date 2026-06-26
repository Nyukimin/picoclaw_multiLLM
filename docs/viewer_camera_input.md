# Viewer Camera Input

## Current Contract

Viewer camera input is browser-side still-frame capture.

The Viewer captures the device camera preview and converts it to JPEG still
frames in the browser. The current continuous capture mode is 1 FPS and stores
the frames as normal Viewer image attachments.

Runtime routing must remain:

```text
picoclaw -> RenCrow_LLM -> LLM backend
```

The Viewer must not call Gemma4, MLX-VLM, Ollama, or other model backends
directly.

## Reason

Current MLX-VLM 0.6.2 chat completions accepts image inputs, but the tested
`video_url` / `input_video` content parts are rejected by schema validation.
For Gemma4 video understanding, the practical input shape is therefore a frame
sequence.

## Future Option

The transport may be changed later to one-second video chunks:

```text
camera stream -> 1s video chunk -> picoclaw -> RenCrow_LLM -> frame extraction or input_video
```

That change should stay behind the same RenCrow routing boundary. The upper
Viewer camera UI should not need to call model backends directly.
