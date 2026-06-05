# Prod GoClaw DB access (everest cluster)

CloudNativePG cluster `goclaw-db-1-1`, namespace `databases`. Tenant for the
Vietnamese/SHTP deployment: `019d542d-5c1f-74e9-9e67-f65044b7445c`.

```bash
cd /Users/vanducng/git/personal/dataplanelabs/infra && eval "$(mise env -s zsh)"   # KUBECONFIG + SOPS
kubectl config current-context        # expect: everest
PSQL()  { kubectl exec    -n databases goclaw-db-1-1 -c postgres -- psql -U postgres -d goclaw -X -t -A "$@"; }
PSQLi() { kubectl exec -i -n databases goclaw-db-1-1 -c postgres -- psql -U postgres -d goclaw -X -q "$@"; }
```
- Use `-U postgres` (peer auth as the OS user). The `goclaw` role fails peer auth; it needs a password over TCP.
- Write large/UTF-8 content safely with base64 over stdin:
  `printf "UPDATE … SET content=convert_from(decode('%s','base64'),'UTF8') WHERE …;" "$(base64 < file | tr -d '\n')" | PSQLi`

## Schema cheatsheet
- `agents(id, agent_key, display_name, model, tenant_id, self_evolve, other_config, tools_config, …)` — identity is dual: UUID for FK/DB, agent_key for paths/logs.
- `channel_instances(name, channel_type, agent_id, …)` — e.g. `zalo-shtp` (zalo_personal) → bound agent.
- `channel_contacts(sender_id, display_name, peer_kind, channel_instance)` — group names: `peer_kind='group'`, `sender_id` = group id used in session_key.
- `sessions(session_key, agent_id, channel, messages jsonb, metadata jsonb, …)` — session_key `agent:<key>:<channel>:group:<id>` | `:cron:<uuid>` | `:direct:<id>`.
- `agent_context_files(agent_id, file_name, content)` / `user_context_files(agent_id, file_name, user_id, content)`. Agent-global memory scope = `user_id IS NULL`. Per-group = `user_id='group:<channel>:<id>'`.
- `memory_documents(agent_id, user_id, path, content, hash, custom_scope)` ; `memory_chunks(document_id→CASCADE, agent_id, user_id, path, start_line, end_line, hash, text, embedding vector(1536) NULL, tsv tsvector GENERATED, tenant_id NOT NULL)`.
- `kg_entities(id, agent_id, user_id, name, entity_type, description, confidence, tenant_id, valid_until)` ; `kg_relations(source_entity_id, target_entity_id, relation_type, agent_id, user_id, tenant_id)` — UNIQUE `(agent_id,user_id,source_entity_id,relation_type,target_entity_id)`.
- Memory search (`internal/store/pg/memory_search.go`) = hybrid: tsv `ts_rank` + vector `embedding <=> q`. tsv works without embeddings.
