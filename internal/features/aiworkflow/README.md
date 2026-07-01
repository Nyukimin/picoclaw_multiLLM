# Aiworkflow Feature

## Owner

SuperAgent / AI Workflow

## Inputs

workflow request, action, trace query, Viewer operation request

## Outputs

workflow status, action result, trace event

## Side Effects

workflow execution through existing viewer/runtime handlers

## Persistence

existing workflow/job logs where configured

## Logs

workflow_id, action, status, error kind

## Error Contract

workflow failure must be visible and must not be shown as stale success

## Current Primary Files

internal/adapter/viewer/ai_workflow_handler.go, internal/application/superagent

## Migration Boundary

This feature package is a registrar/facade entry point only. Existing implementation stays in the listed current files until contract tests and caller handoff are added for the relevant phase.
