# Lingma2API Platform Refactor Plan

> Status: consolidated plan after requirement review
> Date: 2026-05-01
> Scope: align `lingma2api` from a minimal OpenAI-style proxy into a platformized single-account gateway with dual northbound protocol support and canonical internal models.

## 1. Goal

Evolve `lingma2api` into a platformized gateway with these first-class properties:

1. A single global Lingma account at runtime, with data-model reservations for future multi-account expansion.
2. Two formal northbound APIs:
   - OpenAI: `/v1/models`, `/v1/chat/completions`
   - Anthropic: `/v1/messages`
3. A provider-neutral canonical request / session / execution model.
4. A canonical policy engine that governs execution behavior before southbound Lingma request construction.
5. A control plane that treats logs, replay, policy, and session state as canonical resources rather than OpenAI-specific request artifacts.

## 2. Confirmed Product Decisions

### 2.1 Platform positioning

1. `lingma2api` should remain a proxy platform, not a protocol research project.
2. Runtime remains single-account.
3. Multi-account is not part of the first implementation, but schema and control-plane design must leave room for it.

### 2.2 Northbound protocol stance

1. OpenAI and Anthropic are both formal first-class northbound APIs.
2. Anthropic is not an experimental compatibility shim.
3. Control-plane records must always carry protocol metadata such as:
   - `ingress_protocol`
   - `ingress_endpoint`

### 2.3 Canonical internal model

1. Internal modeling must move from an OpenAI-shaped `Message` core to a provider-neutral canonical IR.
2. Canonical turn snapshots must use ordered content blocks.
3. Canonical blocks must preserve non-text structure such as:
   - `image`
   - `document`
   - `tool_call`
   - `tool_result`
   - `reasoning`
4. Southbound Lingma projection may degrade unsupported block types, but canonical storage must preserve them.

### 2.4 Session model

1. Session state is canonical and shared across OpenAI and Anthropic.
2. Session storage should persist canonical turn snapshots, not protocol-specific payloads.
3. Protocol-specific request/response forms are projections, not primary session state.

### 2.5 Execution record model

1. Primary control-plane fact is the canonical execution record.
2. Each execution record should keep:
   - ingress projection
   - pre-policy canonical request
   - post-policy canonical request
   - southbound request summary
   - execution sidecar
3. Replay default must start from `pre-policy canonical request`.
4. Historical execution replay should also be supported from `post-policy canonical request`.
5. Execution sidecar may contain streaming details and raw protocol evidence, but is not the primary session model.

### 2.6 Policy model

1. Policy is a canonical policy engine, not an expanded `model_mappings` table.
2. Policy applies to the canonical request before southbound request building.
3. First version uses declarative policy rules only.
4. First version policy may change execution parameters only.
5. First version policy must not rewrite message content.
6. First version `match` conditions may use canonical normalized attributes only.
7. First version `match` must not depend on raw body, raw headers, JSONPath, or content regex.
8. First version action categories:
   - rewrite model
   - set reasoning
   - allow or deny tools
   - attach tags
9. Evaluation mode:
   - ordered by priority
   - multiple rules may match
   - actions merge by category with constrained winner rules

### 2.7 Resource model

1. New primary control-plane resource: `/admin/policies`
2. Existing `/admin/mappings` becomes a compatibility view for model-rewrite policy only.
3. Existing model-mapping UI and storage should not remain the platform's primary abstraction.

### 2.8 Versioning

1. Canonical request, canonical session, and canonical execution record must carry explicit `schema_version`.
2. Versioned JSON payloads are preferred over relying only on table shape.

## 3. Current Gaps To Close

The current codebase still reflects the earlier minimal-proxy shape in several important places:

1. OpenAI-shaped core request types still dominate body building and request validation.
2. Anthropic handling currently projects into OpenAI-centric structures before southbound build.
3. Session storage currently persists `[]Message` rather than canonical ordered blocks.
4. Logging middleware is centered on `/v1/chat/completions`.
5. Replay is centered on replaying OpenAI chat requests.
6. `model_mappings` is still the primary rule abstraction in DB and frontend.
7. Request logs are stored mostly as downstream/upstream request bodies rather than as canonical execution records.

These are migration targets, not the long-term design.

## 4. Target Architecture

```text
OpenAI Handler ---------\
                         \
                          -> Canonical Request -> Policy Engine -> Session Merge
                         /                                      -> Model Resolve
Anthropic Handler ------/                                       -> Southbound Builder
                                                                 -> Lingma Transport
                                                                 -> Execution Sidecar
                                                                 -> Canonical Execution Record
                                                                 -> Protocol Projection
```

### Core storage layers

1. Canonical session store
2. Canonical execution record store
3. Policy rule store
4. Compatibility view for legacy model mappings

## 5. Work Packages

### Work Package A: Canonical schemas

Define versioned canonical types for:

1. canonical request
2. canonical content block
3. canonical turn snapshot
4. canonical session state
5. canonical execution record
6. execution sidecar

Exit condition:

1. The project has one canonical schema family that no longer assumes OpenAI as the hidden base model.

### Work Package B: Northbound adapters

Refactor northbound handlers so that:

1. OpenAI requests map into canonical request objects.
2. Anthropic requests map into canonical request objects.
3. Validation moves to canonical request validation wherever possible.

Exit condition:

1. Both northbound handlers feed the same canonical request pipeline before policy and southbound build.

### Work Package C: Southbound build split

Separate southbound construction from OpenAI-shaped request types.

Required change:

1. Lingma request building must consume post-policy canonical request and canonical session transcript.

Exit condition:

1. Southbound build no longer depends on OpenAI-shaped internal request structures.

### Work Package D: Canonical session migration

Refactor session storage to:

1. store canonical turn snapshots
2. preserve ordered content blocks
3. share session state across OpenAI and Anthropic

Exit condition:

1. OpenAI and Anthropic can continue a shared session through canonical state.

### Work Package E: Canonical execution record

Introduce a new execution record model with:

1. ingress projection
2. pre-policy canonical request
3. post-policy canonical request
4. southbound request summary
5. execution sidecar
6. protocol metadata
7. `schema_version`

Exit condition:

1. Replay and audit no longer depend on OpenAI-specific stored request bodies as the primary fact model.

### Work Package F: Policy engine

Add:

1. new `policy_rules` storage
2. `/admin/policies`
3. deterministic evaluation and merge behavior
4. rule testing against canonical normalized attributes

Compatibility:

1. `/admin/mappings` remains as a compatibility surface backed by model-rewrite policy data where practical.

Exit condition:

1. Policy becomes the formal control-plane mechanism.

### Work Package G: Control-plane migration

Update admin APIs and frontend so that:

1. logs display protocol-aware execution records
2. replay supports canonical replay semantics
3. policy becomes a first-class UI concept
4. mappings are visually and semantically demoted to compatibility mode

Exit condition:

1. The console reflects platform abstractions rather than legacy minimal-proxy abstractions.

## 6. Replay Semantics

Replay modes to support:

1. `canonical replay`
   - starts from stored pre-policy canonical request
   - re-evaluates current policy
2. `historical execution replay`
   - starts from stored post-policy canonical request
   - reproduces prior execution intent as closely as possible

The default replay mode should be canonical replay.

## 7. First-Version Non-Goals

The following remain explicitly out of scope for this plan's first implementation wave:

1. runtime multi-account scheduling
2. DSL or scripting-based policy rules
3. message-content rewriting policy
4. raw header/body matching in policy
5. prompt gateway behavior
6. protocol-specific session silos

## 8. Acceptance Criteria

This plan should be considered implemented only when all of the following are true:

1. OpenAI and Anthropic enter the same canonical request pipeline.
2. Policy executes on canonical request objects before southbound build.
3. Session state is canonical and shared across both northbound protocols.
4. Canonical turn storage preserves ordered blocks and non-text structure.
5. Execution records store pre-policy and post-policy canonical views with schema versions.
6. Replay defaults to pre-policy canonical replay.
7. `/admin/policies` is the primary rule-management surface.
8. `/admin/mappings` no longer acts as the main platform abstraction.

## 9. Suggested Implementation Order

1. Canonical schemas and versioning
2. Northbound adapters
3. Southbound builder refactor
4. Canonical session migration
5. Canonical execution record storage
6. Policy engine and `/admin/policies`
7. Control-plane and frontend migration

## 10. Notes

1. Existing minimal-proxy and console docs remain useful as implementation history, but this plan should be treated as the new consolidation point for the next refactor wave.
2. Existing `superpowers` documents that describe model mappings or OpenAI-centric logging are now partial, not authoritative, for future platform work.
