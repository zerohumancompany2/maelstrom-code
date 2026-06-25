# Maelstrom Vision Statement

Maelstrom is a foundational framework empowering the creation and operation of **zero-human companies**—fully autonomous, AI-orchestrated organizations that function with minimal or no ongoing human intervention.

## Core Outcome

The end result of our efforts is a world where ambitious individuals (or small teams) can launch and scale sophisticated enterprises that run themselves. These companies handle complex operations, decision-making, customer interactions, development, and growth through coordinated networks of AI agents, intelligent workflows, and dynamic context management—freeing humans to focus on vision, strategy, and high-level direction rather than day-to-day execution.

## What Success Looks Like

- **Autonomous Operations at Scale**: Companies that operate 24/7/365, executing tasks, managing resources, iterating on products/services, and adapting to new challenges with built-in governance, memory, and provenance. Agents and workflows collaborate seamlessly, with durable state persisting across sessions and interruptions.
- **Radical Simplicity and Modularity**: A lightweight, extensible core built around a small set of powerful primitives (session/workflow/agent history, context maps, runtime orchestration). This allows rapid composition of advanced capabilities—like custom memory systems, tool integrations, UI connectors, task scheduling, or external system hooks—without heavy dependencies or bloat.
- **Context-Aware Intelligence**: Agents always have precisely the right information at inference time (via context maps that intelligently partition, manage, and inject data—like repo state, memory, workflow status, or external feeds). No more hallucinated assumptions, missing context, or inefficient tool-chaining; everything is deliberate and efficient.
- **Accessibility and Extensibility**: An approachable framework (initially Go-based MVP) that slots into existing tooling, lowers barriers for builders, and supports everything from solo zero-human ventures to hybrid human-AI orgs. It accelerates the "next 5000 days" of autonomous business by providing durable foundations others can build upon.
- **Transparent, Governed, and Trustworthy Systems**: Full provenance, auditability, and control mechanisms ensure operations are verifiable, secure, and aligned with human intent—paving the way for a new standard of fully transparent companies.

In essence, Maelstrom delivers the infrastructure layer for the zero-human company revolution: turning powerful AI models into reliable, production-grade organizational engines that deliver real economic value with unprecedented efficiency and autonomy.

## Maelstrom Code (MVP) Vision Statement

Maelstrom Code (maelstrom-code) is the lightweight, Go-based core runtime and foundational layer of the Maelstrom framework. It serves as the minimal, high-performance engine that makes **reliable, long-running AI agents and workflows** practical for zero-human and hybrid organizations.

### Core Outcome

The MVP delivers a **simple, understandable, and durable runtime** where developers can define and operate AI agents and multi-step workflows using declarative YAML, powered by statecharts under the hood. The result is autonomous systems that maintain coherent state, intelligently manage context, and execute reliably over long periods—with minimal resource overhead and maximum transparency.

### What Success Looks Like for the MVP

- **Declarative Agent & Workflow Definition**: Agents and workflows are configured primarily through clean, versionable YAML files. This includes agent parameters (model, temperature, context size), cognitive states, and workflow logic modeled as statecharts. No heavy framework boilerplate—just clear, auditable specs that can be hot-reloaded and evolved.
- **Intelligent Context Management**: A powerful context map system that partitions and assembles exactly the right information (repo state, memory, workflow status, external data, etc.) for each inference. This eliminates common agent pitfalls like context bloat, hallucinations from missing info, or inefficient tool use. Context is built deliberately via composable chunks.
- **Minimal & Performant Runtime**: Extremely low footprint (targeting ~8-12MB RAM for sessions), fast startup, and efficient orchestration. Built from scratch in Go for control, reliability, and production readiness in long-running "zero-human" scenarios. Supports durable sessions that survive interruptions.
- **Statechart-Driven Behavior**: Cognitive states (how to think) and workflows (what to think about) are explicitly modeled. Agents can transition states, bind/unbind from workflows, collaborate (e.g., one implements, another reviews), and maintain policy-based tool access. This brings structure and predictability to agentic behavior.
- **Foundation for Extensibility**: A small set of powerful primitives (sessions, context maps, agents, workflows) that serve as the bedrock for higher-level features like memory systems, tool integrations, change control, economic tracking (tokens/power), and governance. Easy to reason about, debug, and build upon.

In short, the maelstrom-code MVP is the **minimal viable engine** that turns the vision of self-sustaining AI organizations into something you can run, inspect, and iterate on today. It prioritizes clarity, efficiency, and durability over complexity—providing the reliable "operating system" layer upon which full zero-human companies can be built and scaled.
