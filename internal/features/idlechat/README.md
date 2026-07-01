# Idlechat Feature

## Owner

IdleChat

## Inputs

idle session config, topic seed, speaker config, stop request, Viewer control event

## Outputs

timeline event, display text, TTS trigger, session status

## Side Effects

LLM calls, TTS enqueue, event emission, topic cache updates through existing services

## Persistence

topic store, session logs, existing idlechat state

## Logs

session_id, topic_id, speaker, route, status, error kind

## Error Contract

fallback and invalid response must not be treated as success

## Current Primary Files

internal/application/idlechat, cmd/picoclaw/runtime_idlechat*.go, cmd/picoclaw/idlechat_tts*.go

## Migration Boundary

This feature package is a registrar/facade entry point only. Existing implementation stays in the listed current files until contract tests and caller handoff are added for the relevant phase.
