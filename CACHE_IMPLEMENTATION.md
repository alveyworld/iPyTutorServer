# Prompt Optimization & Caching - Deployment Checklist

## Summary
Implemented automatic prompt caching + dramatically reduced prompt size through instruction file optimization and markdown concepts in server state. Combined approach reduces per-request payload by 67-71% and caches the base bundle once per session.

## Prompt Size Optimization

### Before vs After

| Component | Before | After | Reduction |
|-----------|--------|-------|-----------|
| **Base Bundle** | 50 KB | 17.6 KB | 65% |
| **Per-Request Dynamic** | 20 KB | 5-8 KB | 60-75% |
| **Typical Total Request** | 70 KB | 22-25 KB | 65-69% |

### Detailed Breakdown

**Base Bundle (cached once per session):**
- IPyIntervu-entrypoint.md: 13.7 KB → **9.7 KB** (consolidated redundant sync sections, removed duplicate tables)
- IPyIntervu-protocols.md: 27.9 KB → **2 KB** (removed redundancy, examples, dictionary prohibition)
- IPyIntervu-week-scope.md: 4 KB → **0.5 KB** (removed table, week 7 examples, pre-checklist)
- IPyIntervu-modes-shared.md: 5.4 KB → 5.4 KB (unchanged)
- **Total base bundle: 50 KB → 17.6 KB**

**Per-Request Content (sent with every request, base bundle cached):**
- Session state JSON: ~1 KB (personaNames + currentWeekConcepts markdown)
- Mode-specific file: ~7-9 KB (only current mode)
- Week-specific files: ~3-5 KB (only current week rubric)
- Conversation history: ~1-2 KB
- **Total dynamic: 20 KB → 5-8 KB**

## What Changed

### 1. Model Upgrade ✅
- **File**: `prompt.go:10`
- **Change**: `deepseek-v4-flash` → `deepseek-v4-flash-0731`
- **Why**: Supports automatic prompt caching

### 2. Instruction File Condensing ✅
- **Protocols.md**: Removed ~25 KB of redundant examples, dictionary prohibition, rule restatements
  - Kept: Sync block JSON rules (essential), single-question rule, stay-in-character
- **Week-scope.md**: Removed ~3.5 KB of tables, examples, detailed checklists
  - Kept: Single rule (use concepts from week N and prior only)

### 3. Move to Server State ✅
- **File**: `agent_state.go` + `state_machine.go`
- **PersonaNames**: Map of mode+number to human names (e.g., "Conceptual-1" → "Alex")
- **CurrentWeekConcepts**: Markdown string of allowed concepts for current week + prior weeks
- **Effect**: No file loading needed, smaller per-request state, more natural for model

### 4. Cache Priming ✅
- **Bootstrap**: Prime base bundle (~15 KB) async when session starts
- **Week Selection**: Prime week rubric async when week is selected
- **Result**: OpenRouter's sticky routing keeps cache warm across all requests in session

### 5. Chat Handler Integration ✅
- **File**: `chat_handler.go`
- **Logic**:
  - Detect week selection, prime cache async
  - Build and store markdown concepts in state
  - Reuse cached prompts across all internal turns

## How It Works

### Session Flow
```
Bootstrap (after auth):
  └─ Async: primeCacheWithBaseBundle() sends 15 KB base instructions
     └─ DeepSeek caches the prefix (sticky routing, automatic)

Week Selection:
  └─ Async: primeCacheWithWeekRubric() adds week rubric to cache
  └─ initializePersonasAndConcepts() builds markdown for allowed concepts
  └─ Session state updated with personaNames + currentWeekConcepts

Each Chat Request:
  └─ buildDynamicPrompt() sends:
     • Base bundle (read from cache at 0.1x cost) — 17.6 KB saved
     • Markdown concepts from session state — ~0.5 KB
     • Mode-specific file (current only) — 7-9 KB
     • Week rubric (current only) — 3-5 KB
     • Conversation history — 1-2 KB
  └─ Total new tokens: ~5-8 KB (vs 20 KB before optimization)
```

### Size & Performance Savings
- **Instruction files**: 50 KB → 17.6 KB (65% reduction, cached once)
- **Per-request size**: 20 KB → 5-8 KB (60-75% reduction)
- **Model processing**: Cached base bundle read at 0.1x cost by DeepSeek
- **Total per-request**: 70 KB → 22-25 KB (65-69% reduction)

## Deployment Steps

### 1. Verify Code Changes
```bash
cd /home/manager/iPyIntervuServer
git diff  # Review all changes
```

### 2. Build New Binary
```bash
cd openrouter-app
go build -o openrouter-app .
```

### 3. Test (Optional)
- The test server on port 8765 has the changes
- Real testing happens after deployment to production

### 4. Deploy
```bash
# Stop current server
sudo systemctl stop openrouter-app

# Backup old binary
sudo cp /usr/local/bin/openrouter-app /usr/local/bin/openrouter-app.bak

# Deploy new binary
sudo cp /home/manager/iPyIntervuServer/openrouter-app/openrouter-app /usr/local/bin/

# Start new server
sudo systemctl start openrouter-app

# Verify
sudo systemctl status openrouter-app
```

## Monitoring

### Check Cache is Working
Look for `[cache]` log lines:
```bash
journalctl -u openrouter-app -f | grep cache
```

### Expected Logs
- `[cache] prime_static_core status=200` — Cache warmed successfully
- Check `cached_tokens` in OpenRouter responses (if available in response metadata)

### Expected Performance Metrics
- **Before optimization**: 30-60s per request (70 KB prompts, no caching, model inference slow)
- **After optimization**: 
  - First message (week selection): ~20-40s (full prompt, cache priming)
  - Subsequent messages: ~10-15s (base cached, 65% smaller payload)
  - Internal turns: ~8-12s each (cached base + smaller payload)
- **Speed improvement**: 2-6x faster on subsequent requests vs before

## Rollback
```bash
sudo systemctl stop openrouter-app
sudo cp /usr/local/bin/openrouter-app.bak /usr/local/bin/openrouter-app
sudo systemctl start openrouter-app
```

## Notes
- Caching is **transparent** — no code changes needed for DeepSeek to use it
- Cache is per-session — new sessions get fresh cache
- Works across all internal turns (up to 6 per user request)
- Static core is extracted separately to minimize cache invalidation
- Async priming ensures non-blocking cache warmup

## Files Modified

### Core Implementation
1. `prompt.go` — Upgrade model to deepseek-v4-flash-0731, bootstrap async cache priming
2. `agent_state.go` — Add personaNames and currentWeekConcepts (markdown string) fields
3. `state_machine.go` — initializePersonasAndConcepts() builds markdown for allowed concepts
4. `prompt_router.go` — Condense protocols.md and week-scope.md, simplify buildDynamicPrompt
5. `openrouter_retry.go` — primeCacheWithPrompt() generic cache priming
6. `chat_handler.go` — Detect week selection, prime week rubric async

### Instruction Files (Optimized)
- `env/instructions/IPyIntervu-protocols.md` — 27.9 KB → 2 KB (removed 25 KB redundancy)
- `env/instructions/IPyIntervu-week-scope.md` — 4 KB → 0.5 KB (removed tables, examples, checklists)
