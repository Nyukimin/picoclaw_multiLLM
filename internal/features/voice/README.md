# Voice Feature

## Owner

Voice

## Inputs

voice websocket request, VDS gateway config, audio route event, voice input mode

## Outputs

voice session event, LLM delta/final, runtime URL plan, bridge availability

## Side Effects

websocket streaming, bridge calls, audio event emission through existing runtime code

## Persistence

existing voice session/runtime state only

## Logs

session_id, utterance_id, route, input mode, status, error kind

## Error Contract

voice disabled, session mismatch, LLM busy, and provider failure are explicit

## Current Primary Files

modules/voicechat, cmd/picoclaw/voice_chat_runtime_*.go, internal/adapter/viewer/audio_router_sse.go

## Migration Boundary

This feature package is a registrar/facade entry point only. Existing implementation stays in the listed current files until contract tests and caller handoff are added for the relevant phase.
