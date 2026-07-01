# Reports Feature

## Owner

Reports / Evidence

## Inputs

claim, evidence item, verification request, report query

## Outputs

verification report, evidence summary, status, export artifact

## Side Effects

verification report writes and artifact export through existing services

## Persistence

verification persistence store and report artifacts

## Logs

report_id, claim_id, evidence_id, status, error kind

## Error Contract

search and unverified evidence remain distinct from browser/source evidence

## Current Primary Files

internal/application/verification, internal/domain/verification, internal/infrastructure/persistence/verification, internal/adapter/viewer/verification_handler.go

## Migration Boundary

This feature package is a registrar/facade entry point only. Existing implementation stays in the listed current files until contract tests and caller handoff are added for the relevant phase.
