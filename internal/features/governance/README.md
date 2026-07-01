# Governance Feature

## Owner

Governance

## Inputs

skill change, manifest, coder evidence, external PR request

## Outputs

gate decision, trigger log, audit report, bootstrap result

## Side Effects

governance store writes and external PR preparation through existing services

## Persistence

skill governance persistence store

## Logs

skill_id, change_id, gate, status, error kind

## Error Contract

unreviewed external contributions must not bypass governance gates

## Current Primary Files

internal/application/skillgovernance, internal/domain/skillgovernance, internal/infrastructure/persistence/skillgovernance, internal/adapter/viewer/skill_governance_handler.go

## Migration Boundary

This feature package is a registrar/facade entry point only. Existing implementation stays in the listed current files until contract tests and caller handoff are added for the relevant phase.
