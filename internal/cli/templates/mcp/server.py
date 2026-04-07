"""
Model Context Protocol (MCP) Server Template for {{NAME}}

This module implements an MCP server that exposes tools and resources
to Claude and other AI assistants.
"""

import json


class {{NAME|title|replace('-', '')}}Server:
    """{{NAME}} MCP Server Implementation"""
    
    def __init__(self):
        self.name = "{{NAME}}"
        self.version = "0.1.0"
        self.description = "{{DESCRIPTION}}"
    
    def get_tools(self) -> list:
        """
        Return available tools for this MCP server.
        
        Each tool should define:
        - name: Tool identifier
        - description: What the tool does
        - input_schema: JSON schema for inputs
        """
        return [
            {
                "name": "example_tool",
                "description": "An example tool from {{NAME}}",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "input": {"type": "string"}
                    },
                    "required": ["input"]
                }
            }
        ]
    
    def get_resources(self) -> list:
        """
        Return available resources this server exposes.
        
        Resources can be documents, data, or other structured information.
        """
        return []
    
    def execute_tool(self, tool_name: str, args: dict) -> str:
        """Execute a tool and return the result."""
        if tool_name == "example_tool":
            return f"Tool executed with input: {args.get('input')}"
        return "Unknown tool"


# Server metadata
SERVER_METADATA = {
    "name": "{{NAME}}",
    "description": "{{DESCRIPTION}}",
    "version": "0.1.0",
    "author": "{{AUTHOR}}",
    "type": "mcp_server",
}
