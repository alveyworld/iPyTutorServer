# OpenRouter Retry and Delay Statistics

**Analysis Date:** 2026-09-29  
**Log Period:** Last 5000 systemd journal entries  
**Data Source:** systemd journal from openrouter-app service  
**Total Log Entries Analyzed:** 2,498 OpenRouter operations  

## Executive Summary

Analysis of 223 first-turn API requests to OpenRouter shows that while the first attempt has an 86.1% success rate, implementing a 3-attempt retry strategy with exponential backoff brings overall reliability to **98.2%**, with only 1.8% permanent failure rate.

## Success Rates by Attempt

### Attempt 1 (Initial Request)
- **Starts:** 223
- **Success:** 192
- **Success Rate:** 86.1%
- **Failed (needed retry):** 31

### Attempt 2 (First Retry)
- **Starts:** 31 (only failed requests)
- **Success:** 21
- **Success Rate:** 67.7% (of retry attempts)
- **Failed:** 10

### Attempt 3 (Second Retry)
- **Starts:** 10 (only twice-failed requests)
- **Success:** 6
- **Success Rate:** 60.0% (of retry attempts)
- **Permanently Failed:** 4

## Cumulative Reliability

| After N Attempts | Cumulative Success | Success Rate |
|------------------|-------------------|--------------|
| 1 attempt        | 192/223           | 86.1%        |
| 2 attempts       | 213/223           | 95.5%        |
| 3 attempts       | 219/223           | **98.2%**    |

**Retry Improvement:** +12.1% absolute (from 86.1% to 98.2%)

## Retry Recovery Rates

- **2nd attempt recovers:** 67.7% of first-attempt failures (21 out of 31)
- **3rd attempt recovers:** 60.0% of second-attempt failures (6 out of 10)
- **Permanent failure rate:** 1.8% (only 4 requests after exhausting all retries)

## Response Timing Analysis

### Average Response Times by Attempt

| Attempt | Average Elapsed Time |
|---------|---------------------|
| 1st     | ~30 seconds          |
| 2nd     | ~56 seconds          |
| 3rd     | ~66 seconds          |

**Note:** The increase in response time on retries suggests:
1. Exponential backoff delays (1.5s after attempt 1, 3s after attempt 2)
2. OpenRouter may be processing more complex/longer requests on retries
3. Subsequent attempts are seeing higher latency from OpenRouter

### Retry Delay Configuration

```
Base Delay: 1500 ms
Attempt 1 → Attempt 2: 1500 ms wait
Attempt 2 → Attempt 3: 3000 ms wait (2x multiplier)
```

## Failure Analysis

### Primary Failure Reason

**All 54 detected failures** were classified as `openrouter_timeout`:
- **Failure Point:** `upstream_read` phase
- **Root Cause:** Timeout reading response body from OpenRouter API
- **Error Message:** "context deadline exceeded (Client.Timeout or context cancellation while reading body)"

### Failure Distribution

- OpenRouter timeouts: 54 instances
- Other error classes: 0 detected
- Connection errors: 0
- Bad gateway responses: 0

**Key Insight:** 100% of failures are network/upstream timeouts, indicating OpenRouter is experiencing latency or the response timeout threshold may be too aggressive.

## Response Size Analysis

### Average Response Body Size by Attempt

| Attempt | Average Body Size |
|---------|------------------|
| 1st     | ~10,363 bytes    |
| 2nd     | ~15,347 bytes    |
| 3rd     | ~16,286 bytes    |

Larger responses on retries may be due to:
- Retried requests including more context
- Different prompts being sent on subsequent attempts
- Requests that timeout on first attempt being more complex

## Multi-Turn Request Analysis

- **Single-turn requests:** 177 (mode_turn=0 only)
- **Multi-turn requests:** 46 (mode_turn ≥ 1)
- **Analysis scope:** This report focuses on first-turn (mode_turn=0) success rates

## Key Findings

1. **✓ The 3-attempt retry strategy is highly effective**
   - Brings reliability from 86.1% to 98.2%
   - Each subsequent attempt still recovers 60%+ of failures

2. **✓ Timeouts are the sole failure mode**
   - 100% of failures are `upstream_read` timeouts
   - No transport errors, bad gateways, or connection resets

3. **⚠ First-attempt success is relatively low**
   - 86.1% is below industry standard for API reliability
   - Suggests OpenRouter may be experiencing consistent latency

4. **⚠ Exponential backoff helps but doesn't fully solve the problem**
   - 2nd attempt still fails 32.3% of the time
   - 3rd attempt still fails 40.0% of the time
   - Indicates systemic OpenRouter latency, not transient glitches

5. **✓ User impact is effectively mitigated**
   - 98.2% overall success = only 4 users out of 223 experience failure
   - Acceptable for an assessment/interview tool

## Configuration Details

### Current Retry Configuration (from openrouter_retry.go)

```go
const (
    openRouterMaxAttempts = 3
    openRouterRetryBase   = 1500 * time.Millisecond
)

func openRouterRetryDelay(attempt int) time.Duration {
    return openRouterRetryBase * time.Duration(attempt+1)
}
```

### Retryable Error Types

The system retries on:
- `io.EOF` and `io.ErrUnexpectedEOF`
- Network timeouts and temporary errors
- `context.DeadlineExceeded`
- Connection resets (`syscall.ECONNRESET`)
- Broken pipes (`syscall.EPIPE`)

### Retryable HTTP Status Codes

- 429 (Too Many Requests)
- 502 (Bad Gateway)
- 503 (Service Unavailable)
- 504 (Gateway Timeout)

*Note: None of these status codes were encountered in the analyzed period.*

## Historical Context

From commit `12eab9d` (2026-06-29):
> "changed from streaming responses from openrouter to full responses. Stability in an assessment tool is more important than gradually seeing the response"

This indicates that:
- Previous streaming implementation had reliability issues
- Full response fetching was chosen for stability
- Current retry strategy is a continuation of stability-first philosophy

## Recommendations

### 1. Monitor and Track

- **Continue logging retries** at current verbosity level
- **Track by model/provider** — different OpenRouter models may have different reliability
- **Alert on retry spikes** — if retry rates suddenly increase, may indicate OpenRouter issues

### 2. Potential Improvements

- **Increase context deadline** — If timeouts are transient, extending deadline might help
- **Add circuit breaker** — If OpenRouter is consistently slow, consider temporary fallback
- **Implement adaptive backoff** — If specific models are slower, use model-specific delays
- **Request timeout headers** — Check if OpenRouter supports custom timeout parameters

### 3. User Facing

- **Current 98.2% reliability is acceptable** for an assessment tool
- **Consider showing user feedback** on retries (e.g., "Retrying..." message)
- **Log failures for support** — The 1.8% that fail need investigation

### 4. Operations

- **Set up alerting** on failures exceeding threshold
- **Weekly review** of retry statistics
- **Correlate with OpenRouter incidents** — Check if spikes align with their outages

## Conclusion

The OpenRouter retry and delay strategy is **working well**. The combination of:
- 3 maximum attempts
- Exponential backoff (1.5s, 3s delays)
- Aggressive retry on all network errors

Results in a **98.2% success rate** for users, with acceptable latency (~30-66 seconds per request depending on retries).

The main takeaway is that OpenRouter appears to be experiencing some baseline latency/timeout issues (86% first-attempt success), but the retry strategy effectively compensates for this, leaving minimal impact on end users.
