"""
Hook Template for {{NAME}}

This module defines a hook that is executed at specific points 
in the Summon lifecycle or AI assistant workflow.
"""


---
name: {{NAME}}
description: "A {{NAME}} package for Summon"
---

# {{NAME}} Hook

Describe when this hook is triggered and what it does.

## Trigger Events

- List when this hook executes
- Describe conditions that activate it

## Actions

Describe what this hook does when triggered.

## Context

Describe what data or context is available to this hook.


# Hook metadata
HOOK_METADATA = {
    "name": "{{NAME}}",
    "description": "{{DESCRIPTION}}",
    "version": "0.1.0",
    "author": "{{AUTHOR}}",
    "triggers": [
        # Examples: "on_install", "on_activate", "on_deactivate"
        # Add the appropriate trigger points for your hook
    ],
}
