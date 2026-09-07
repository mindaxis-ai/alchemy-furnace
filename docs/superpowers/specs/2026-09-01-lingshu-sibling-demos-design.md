# Lingshu sibling projects: minimum demos

## Purpose

Define two independently usable, local-first Lingshu desktop demos. They must complement, but never become runtime dependencies of, Alchemy Furnace.

Lingshu is the umbrella brand:

> Lingshu — Local-first tools for shaping AI minds.

Its first products are Alchemy Furnace (炼丹炉), Bencao (百草堂), and Zangjingge (藏经阁).

## Product boundaries

| Product | First-class object | Responsibility | Explicit non-goal |
|---|---|---|---|
| Alchemy Furnace | Golden pill / Skill | Compose, run, and observe skills and personas | Training and document retrieval infrastructure |
| Bencao | Behaviour dataset and experiment | Create examples, fine-tune, and evaluate persona-plus-skill behaviour | RAG, document chunking, and a generic knowledge base |
| Zangjingge | Source document and citation | Ingest local documents and answer with attributable evidence | Fine-tuning and persona training |

Every product owns its own local application-data directory and SQLite database. No database is shared and no product requires another one to be running.

## Bencao minimum demo

### Learning question

Can a local LoRA adapter make a small open model follow a defined persona-plus-skill behaviour more consistently than the base model?

### Demo flow

1. Create one training task containing a fictional or consented persona, one named golden pill, and a short target-behaviour statement.
2. Create thirty hand-authored instruction/response examples.
3. Validate roles, required fields, duplicates, and train/validation/test split; export JSONL.
4. Run LoRA training locally.
5. Execute ten held-out prompts against the base model and adapter, then record a human winner for each prompt.

### User interface

- Training task: persona, pill name, and behaviour target.
- Dataset: editable examples and split validation.
- Refinement: training configuration, streamed subprocess state, and artifact path.
- Evaluation: side-by-side base/adapter response comparison and a winner selector.

### Technical foundation

- Platform: macOS Apple Silicon, M1 with 16 GB unified memory.
- Training and inference: open-source MLX-LM.
- Initial model: a quantized Qwen 0.6B or 1.7B instruct model; model selection remains configurable.
- Storage: SQLite metadata plus project-local JSONL and artifact directories.
- Training output: adapter weights, adapter configuration, dataset snapshot hash, command/configuration, and evaluation results.

MLX-LM is invoked as a subprocess, not reimplemented. The demo uses only the documented chat JSONL form and LoRA command surface.

### Non-goals

- Cloud training, Docker, user accounts, team datasets, preference optimization, automatic synthetic-data generation, adapter merging, and multi-adapter composition.
- Claims that a small adapter creates genuine human consciousness or accurately replicates a real person.

## Zangjingge minimum demo

### Learning question

Can a local retrieval flow answer questions from supplied material while showing the exact source excerpt used?

### Demo flow

1. Import up to three Markdown or plain-text files.
2. Parse, chunk, embed, and persist a local index.
3. Ask a question.
4. Return an answer and the retrieved excerpts, filenames, and chunk locations.
5. Rebuild or delete the local index.

### User interface

- Scriptures: local-file import and ingestion status.
- Ask: question, answer, and visible citations.
- Index: document list, chunk counts, rebuild, and delete actions.

### Technical foundation

- Platform: macOS Apple Silicon, M1 with 16 GB unified memory.
- Retrieval orchestration: open-source LlamaIndex.
- Local inference: Ollama with a small open-weight generation model and local embedding model.
- Storage: project-local SQLite index and application metadata.
- First release inputs: `.md` and `.txt` only.

### Non-goals

- PDF/OCR, browser crawling, cloud synchronization, external-vector-database hosting, user permissions, or automatic truth guarantees.

## Interoperability

The demos must work without interoperability enabled. Future collaboration is file- and contract-based:

- Alchemy Furnace can export a read-only Skill description as a target-behaviour reference for Bencao.
- Bencao can export an adapter manifest; Alchemy Furnace may later opt to load it for comparison with a prompt implementation.
- Zangjingge may later expose retrieved, cited context through a local interface that Alchemy Furnace can opt into.

No shared database, forced HTTP dependency, or automatic background synchronization is allowed.

## Brand direction

Produce three related square marks with transparent backgrounds and no text: Lingshu, Bencao, and Zangjingge. They must reference the existing Alchemy Furnace mark through spare black ink-like linework, balanced white space, circular forms, and a calm classical-Chinese visual language.

- Lingshu: an abstract central pivot/ring with a small rising spark, representing the system that connects mind, data, and knowledge.
- Bencao: a round medicine bowl or mortar, an abstract herb sprout, and a small pill.
- Zangjingge: a compact open scroll and archive-pavilion form, with a subtle rising stroke to echo the furnace smoke.

Avoid literal Chinese or Latin lettering, gradients, photorealism, seals, crowded ornament, watermarks, and color backgrounds.

## Acceptance criteria

1. Each README describes the product boundary, the one learning question, the one local demo flow, setup assumptions, open-source dependencies, and non-goals.
2. Each demo is independently runnable on an M1 Mac with 16 GB unified memory; its first loop uses only local files and open-source model tooling.
3. Each generated logo is an original, square, transparent-background PNG with no text and consistent family resemblance to the Furnace mark.
4. The existing Alchemy Furnace code and product data are not modified by this documentation-and-brand-asset task.
