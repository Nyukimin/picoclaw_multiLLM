# Knowledge Feature

## Owner

Knowledge

## Inputs

validated source item, import request, wiki page/index request, glossary input

## Outputs

knowledge item, wiki index, import report, glossary state

## Side Effects

knowledge store writes and wiki artifact updates through existing services

## Persistence

knowledge DB/wiki stores and existing L1 records

## Logs

source_id, item_id, domain, import status, error kind

## Error Contract

unreviewed discovery must not be treated as confirmed knowledge

## Current Primary Files

internal/application/knowledge, internal/application/knowledgememory, internal/adapter/viewer/knowledge_memory_handler.go, cmd/picoclaw/cli_knowledge.go

## Migration Boundary

This feature package is a registrar/facade entry point only. Existing implementation stays in the listed current files until contract tests and caller handoff are added for the relevant phase.
