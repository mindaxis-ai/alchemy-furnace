# Tasks: 金丹化性 · Skill-Persona Alchemy Pivot

**Input**: Design documents from `/specs/001-skill-persona-alchemy-pivot/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Not explicitly requested; test tasks are included only for critical integration points.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prepare branch, tool config, and project skeleton for the pivot.

- [X] T001 Create feature branch `001-skill-persona-alchemy-pivot` from `master`
- [X] T002 Update `.env.example` to remove Qdrant variables and add `PYTHON_ENGINE_BASE_URL`, `SYNTHESIS_MODEL` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/.env.example`
- [X] T003 [P] Update `docker-compose.yml` to remove `qdrant` service and health check references in `/Users/yaoyuliang/ai_coding/alchemy-furnace/docker-compose.yml`
- [X] T004 [P] Update `Makefile` to remove RAG/vector commands and add synthesis/test commands in `/Users/yaoyuliang/ai_coding/alchemy-furnace/Makefile`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core data model and service contracts that MUST be complete before ANY user story can be implemented.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Data Model Foundation

- [X] T005 [P] Add `skill_schema`, `tags`, `author`, `version`, `is_builtin` to `ElixirPill` and remove `status`, `vector_count` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/model/models.go`
- [X] T006 [P] Add `weight`, `sort_order` to `AgentPill` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/model/models.go`
- [X] T007 [P] Create `LanguagePattern` model with `agent_id`, `system_prompt`, `emergence_rules`, `inner_tensions`, `source_fingerprint`, `is_valid` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/model/models.go`
- [X] T008 [P] Remove `ElixirRecipe` model and related request DTOs from `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/model/models.go`
- [X] T009 Update Go request/response DTOs (`CreatePillRequest`, `UpdatePillRequest`, `CreateAgentRequest`, etc.) to match new schema in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/model/models.go`

### Configuration & Shared Clients

- [X] T010 Rename `PythonRAG` config to `PythonEngine` and remove Qdrant config in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/pkg/config/config.go`
- [X] T011 Create `SynthesisClient` in Go for calling Python synthesis API in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/synthesis_client.go`
- [X] T012 Add `LanguagePatternService` in Go to cache/invalidate synthesized prompts in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/language_pattern_service.go`

### Python Language Engine Skeleton

- [X] T013 Create `LanguageSynthesisService` in Python with `combine()` method signature in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/language_synthesis_service.py`
- [X] T014 Add `/api/v1/synthesis/combine` endpoint in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/api/synthesis.py`
- [X] T015 Update Python `ChatService` to accept pre-built system prompt instead of retrieving context in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/chat_service.py`
- [X] T016 Remove vector/retrieval/embedding modules from `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/core/`
- [X] T017 Update Python `schemas.py` to replace vector/RAG schemas with synthesis and chat schemas in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/models/schemas.py`

**Checkpoint**: Foundation ready - database models, Go clients, and Python synthesis skeleton exist; user story implementation can now begin in parallel.

---

## Phase 3: User Story 1 - Create Taoist and Consume Pills (Priority: P1) 🎯 MVP

**Goal**: Users can create a `DaoAgent`, configure base personality, and bind one or more `ElixirPill`s; the agent's replies reflect the blended language pattern.

**Independent Test**: Create a Taoist with personality "沉稳" → bind "文言文金丹" → send a message → reply should be calm and classical.

### Models & Data Access

- [X] T018 [US1] Update GORM auto-migration in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/dao/migrate.go` (or equivalent) to include new `LanguagePattern` table and modified `ElixirPill`/`AgentPill` columns
- [X] T019 [US1] Update `AgentService.GetAgent` to preload `AgentPills` with weights/sort_order and `LanguagePattern` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/agent_service.go`
- [X] T020 [US1] Update `AgentService.BindPill` to accept `weight`/`sort_order` and invalidate language pattern cache in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/agent_service.go`
- [X] T021 [US1] Update `AgentService.UnbindPill` to invalidate language pattern cache in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/agent_service.go`

### Synthesis Integration

- [X] T022 [US1] Implement structured merge logic for `expression_dna`, `mental_models`, `heuristics`, `taboos` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/language_synthesis_service.py`
- [X] T023 [US1] Implement LLM emergence derivation prompt to generate final system prompt + rules in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/language_synthesis_service.py`
- [X] T024 [US1] Compute `source_fingerprint` SHA256 from personality + sorted pills + weights in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/language_synthesis_service.py`
- [X] T025 [US1] Implement Go `LanguagePatternService.GetOrBuildPattern` that calls Python `/api/v1/synthesis/combine` and caches result in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/language_pattern_service.go`

### Chat Flow

- [X] T026 [US1] Update Go `ChatService.CallRAGStream` to call Python `/api/v1/chat/completions/stream` with synthesized system prompt in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/chat_service.go`
- [X] T027 [US1] Update Go WebSocket chat handler to load language pattern before streaming in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/handler/chat_handler.go`
- [X] T028 [US1] Update Python `chat_completion_stream` to skip retrieval and use provided system prompt in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/chat_service.py`

### Frontend

- [X] T029 [P] [US1] Update Agent create/edit form to support base personality text and model selection in `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/AgentFormPage.tsx` (or equivalent)
- [X] T030 [P] [US1] Update Agent detail page to show consumed pills with weight/sort controls in `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/AgentDetailPage.tsx` (or equivalent)
- [X] T031 [US1] Update chat page to remove source citations UI in `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/ChatPage.tsx` (or equivalent)

**Checkpoint**: User Story 1 should be fully functional - create Taoist, bind pill(s), chat, and observe language pattern change.

---

## Phase 4: User Story 2 - Refine Skill Pills (Priority: P1)

**Goal**: Users can create/edit `ElixirPill`s using the nuwa-skill schema; pills are persisted and reusable.

**Independent Test**: Create a "鲁迅风" pill via UI/API → bind to a Taoist → reply should show sharp, satirical, concise style.

### Backend

- [X] T032 [US2] Update `PillService.CreatePill` to accept and persist `skill_schema`, `tags`, `author`, `version` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/pill_service.go`
- [X] T033 [US2] Update `PillService.UpdatePill` to update new fields and invalidate cached language patterns for consuming agents in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/pill_service.go`
- [X] T034 [US2] Remove vector deletion logic from `PillService.DeletePill` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/pill_service.go`
- [X] T035 [US2] Update `PillService.ListPills` to support `is_builtin` filter in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/pill_service.go`
- [X] T036 [US2] Update pill handler routes to bind new request/response DTOs in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/handler/pill_handler.go`

### Python Quality Check (Optional V1)

- [X] T037 [P] [US2] Implement `/api/v1/quality/validate-pill` endpoint to check required skill_schema fields in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/api/quality.py`

### Frontend

- [X] T038 [P] [US2] Create Pill editor page with form fields mapping to nuwa-skill schema sections in `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/PillEditorPage.tsx`
- [X] T039 [P] [US2] Create Pill list page with search/filter in `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/PillListPage.tsx`
- [X] T040 [US2] Add "from pill to agent" quick-bind action in Pill list/detail pages in `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/PillListPage.tsx`

**Checkpoint**: User Stories 1 AND 2 should both work independently - create pills, create agents, bind, chat.

---

## Phase 5: User Story 3 - Multi-Pill Emergence Effects (Priority: P2)

**Goal**: When a Taoist consumes multiple pills, replies show emergent blended traits and surface severe conflicts as `inner_tensions`.

**Independent Test**: Bind "文言文金丹" + "赛博朋克金丹" to one Taoist → reply should mix classical diction with cyberpunk imagery; `inner_tensions` should contain at least one conflict record.

### Synthesis Engine

- [X] T041 [US3] Implement conflict detection across `expression_dna` dimensions (formality, sentence_length, etc.) in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/language_synthesis_service.py`
- [X] T042 [US3] Add `inner_tensions` output to synthesis response with severity levels in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/language_synthesis_service.py`
- [X] T043 [US3] Enhance emergence derivation prompt to explicitly ask LLM for 2-3 emergent rules when N>=2 pills in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/language_synthesis_service.py`
- [X] T044 [US3] Store `inner_tensions` and `emergence_rules` in `LanguagePattern` cache in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/language_pattern_service.go`

### Frontend

- [X] T045 [US3] Display "丹性相冲" warnings on Agent detail page when `inner_tensions` exist in `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/AgentDetailPage.tsx`
- [X] T046 [US3] Show consumed pills as draggable sortable list to control `sort_order` in `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/AgentDetailPage.tsx`

**Checkpoint**: Multi-pill combinations produce observable emergent style changes and conflict visualization.

---

## Phase 6: User Story 4 - Remove RAG Features (Priority: P2)

**Goal**: RAG-related code, data, and UI are removed or deprecated; deployment no longer depends on Qdrant.

**Independent Test**: Run `docker-compose up` without Qdrant; verify no RAG endpoints are registered; frontend has no document upload入口.

### Backend Cleanup

- [X] T047 [US4] Delete `ElixirRecipe` service file `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/service/recipe_service.go`
- [X] T048 [US4] Remove recipe/upload handlers from `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/handler/`
- [X] T049 [US4] Remove recipe/upload API routes from `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/cmd/server/main.go` (or router file)
- [X] T050 [US4] Delete Python vector service `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/vector_service.py`
- [X] T051 [US4] Remove document/media extraction endpoints and services from `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/api/` and `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/services/`
- [X] T052 [US4] Drop `elixir_recipes` table via migration in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/dao/migrate.go`

### Frontend Cleanup

- [X] T053 [US4] Remove recipe/document upload pages and components from `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/pages/` and `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/components/`
- [X] T054 [US4] Remove RAG-related menu items and routes from `/Users/yaoyuliang/ai_coding/alchemy-furnace/frontend/src/App.tsx` (or router)

### Deployment

- [X] T055 [US4] Remove Qdrant volume and service references from `/Users/yaoyuliang/ai_coding/alchemy-furnace/docker-compose.yml`
- [X] T056 [US4] Update `README.md` to remove RAG-specific descriptions and feature list in `/Users/yaoyuliang/ai_coding/alchemy-furnace/README.md`

**Checkpoint**: RAG stack fully removed; system runs without Qdrant; frontend focused on Taoist/Pill/Chat.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories.

- [X] T057 [P] Seed 3-5 builtin example pills (文言文, 赛博朋克, 鲁迅风, etc.) via migration or seed script in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/dao/seed.go`
- [X] T058 [P] Add integration test for end-to-end Taoist creation + pill binding + chat in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/tests/integration/chat_flow_test.go`
- [X] T059 [P] Add Python unit test for synthesis service with 1-pill and 2-pill fixtures in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/tests/test_language_synthesis_service.py`
- [X] T060 Update health check endpoints to remove Qdrant status in `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/go/handler/health_handler.go` and `/Users/yaoyuliang/ai_coding/alchemy-furnace/backend/python/app/api/health.py`
- [X] T061 Update `CLAUDE.md` and feature docs if architecture changes during implementation in `/Users/yaoyuliang/ai_coding/alchemy-furnace/CLAUDE.md`
- [X] T062 Run quickstart.md validation steps end-to-end in local environment

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories.
- **User Stories (Phase 3-6)**: All depend on Foundational phase completion.
  - User Story 1 and User Story 2 can proceed in parallel after foundation.
  - User Story 3 depends on synthesis engine from User Story 1 and pill schema from User Story 2.
  - User Story 4 (RAG removal) can proceed in parallel with story implementation, but should be validated after stories work.
- **Polish (Phase 7)**: Depends on all desired user stories being complete.

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2). No dependencies on other stories. **MVP**.
- **User Story 2 (P1)**: Can start after Foundational (Phase 2). No dependencies on US1, but naturally integrates with it.
- **User Story 3 (P2)**: Depends on US1 (synthesis flow) and US2 (pills exist). Can start once those foundations exist.
- **User Story 4 (P2)**: Can start after Foundational. Best done in parallel with implementation to avoid conflicts, but final cleanup must happen after US1/US2 are stable.

### Within Each User Story

- Models before services
- Services before handlers/endpoints
- Core implementation before frontend integration
- Story complete before moving to next priority

### Parallel Opportunities

- T002-T004 (env/docker/Makefile) can run in parallel.
- T005-T009 (Go model changes) can run in parallel with T010-T017 (Go config/Python skeleton).
- Within US1: T018-T021 (agent service) and T022-T025 (synthesis) can be developed in parallel until integration.
- Within US2: T032-T036 (backend) and T038-T040 (frontend) can run in parallel.
- US1 and US2 can be developed in parallel after Phase 2.
- T057-T059 (seeds/tests) in Polish phase can run in parallel.

---

## Parallel Example: User Story 1

```bash
# Launch model and service tasks together:
Task: "Update AgentService.GetAgent to preload AgentPills with weights/sort_order in backend/go/service/agent_service.go"
Task: "Implement structured merge logic for expression_dna in backend/python/app/services/language_synthesis_service.py"

# Launch frontend and backend integration in parallel once services are ready:
Task: "Update Agent detail page to show consumed pills with weight/sort controls"
Task: "Update Go WebSocket chat handler to load language pattern before streaming"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1 (create Taoist + consume pill + chat)
4. **STOP and VALIDATE**: Test end-to-end chat with one pill.
5. Deploy/demo if ready.

### Incremental Delivery

1. Setup + Foundational → Foundation ready.
2. User Story 1 → Test independently → Deploy/Demo (MVP!).
3. User Story 2 → Test independently → users can create reusable pills.
4. User Story 3 → Test independently → multi-pill emergence.
5. User Story 4 → Test independently → RAG fully removed, deployment simplified.
6. Polish → seeds, tests, docs.

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together.
2. Once Foundational is done:
   - Developer A: User Story 1 (synthesis + chat flow)
   - Developer B: User Story 2 (pill CRUD + frontend editor)
   - Developer C: User Story 4 (RAG cleanup + docker)
3. After US1/US2 stabilize:
   - Developer A + B: User Story 3 (multi-pill emergence)
4. Stories integrate independently; final Polish phase together.

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence
