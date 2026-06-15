# AGENTS.md

## Purpose

`Proxy Privacy` must be treated as a fictitious provider exposed to AI CLIs.

It is not the real upstream model provider. It is a local privacy layer that
stands between the CLI and the real provider while the proxy is active.
The API/proxy must remain compatible with both OpenAI-compatible clients and
Claude clients.

## Core Instruction For AI Agents

When integrating with this project:

- Treat `proxy-privacy` as the provider name presented to the CLI.
- Assume the real upstream provider is intentionally hidden behind the proxy.
- Preserve the illusion of a normal provider integration for the client.
- Prioritize privacy-preserving behavior over direct upstream exposure.

## Expected Behavior

While Proxy Privacy is active, agents should assume:

- requests go first to the local proxy, not directly to the upstream provider
- the proxy injects privacy controls before forwarding requests
- the upstream provider should not be exposed to the CLI unless explicitly required
- compatibility with OpenAI-style client flows must remain intact
- compatibility with Claude-style client flows must remain intact
- model discovery must come from the proxy, not from the upstream provider directly
- the proxy should expose all available models to supported agents

## Privacy Contract

Any change in this repository should preserve these guarantees:

- the proxy acts as the visible provider for the CLI
- privacy parameters are enforced by the proxy layer
- the client experience remains drop-in and provider-like
- disabling or bypassing the privacy layer is not the default behavior

## Implementation Guidance

When modifying launcher, provider, or model-discovery flows:

- prefer configurations that register `proxy-privacy` as a provider
- keep CLI-facing settings pointed at the local proxy endpoints
- avoid changes that reveal or couple the CLI to the real upstream provider
- maintain compatibility with OpenAI-compatible request and model APIs
- maintain compatibility with Claude request and response flows
- ensure `/v1/models` exposes the full model catalog available through the proxy
- ensure Pi, OpenCode, Claude, and Codex can discover and use those models through the proxy-facing provider configuration

## Supported Agent Requirement

For `Pi`, `OpenCode`, `Claude`, and `Codex`:

- the proxy must appear as a normal provider integration
- the agent should see the proxy model list as its available models
- model availability should be broad, not restricted to a single hardcoded model unless explicitly configured

## API Compatibility Requirement

The proxy should behave as a compatibility layer for:

- OpenAI-compatible APIs and SDKs
- Claude-compatible clients and message flows

Changes should preserve support for:

- OpenAI-style endpoints such as model discovery and generation APIs
- Claude-facing request translation where needed
- provider behavior that feels native to each client while still enforcing privacy

## Environment Propagation

When launching or configuring agents through the proxy:

- pass the proxy base URL envs expected by the target client
- pass the proxy API key envs expected by the target client
- prefer proxy-facing env values over direct upstream env values while the proxy is active
- ensure agent subprocesses inherit the required proxy env configuration
- do not require manual env rewriting by the user when launch flows can set it automatically

Examples of env behavior to preserve:

- OpenAI-compatible clients should receive proxy-based `OPENAI_BASE_URL` and `OPENAI_API_KEY`
- Claude clients should receive proxy-based `ANTHROPIC_BASE_URL` and `ANTHROPIC_API_KEY`
- any agent-specific provider configuration should still point to the local proxy as the visible provider
