package api

func integrationsOpenAPI(paths, schemas map[string]any) {
	schemas["CalendarConnection"] = map[string]any{"type": "object", "properties": map[string]any{
		"provider":   map[string]any{"type": "string"},
		"status":     map[string]any{"type": "string", "enum": []string{"pending", "connected", "error", "reauthorization_required", "disconnected"}},
		"last_sync":  map[string]any{"type": "string", "format": "date-time", "nullable": true},
		"last_error": map[string]any{"type": "string"}, "pending_events": map[string]any{"type": "integer"},
	}}
	schemas["Integrations"] = map[string]any{"type": "object", "properties": map[string]any{
		"connections":       map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/CalendarConnection"}},
		"yandex_configured": map[string]any{"type": "boolean"},
	}}
	for path, method := range map[string]string{"/api/integrations": "get", "/api/integrations/yandex/sync": "post", "/api/integrations/yandex": "delete"} {
		status := "200"
		success := map[string]any{"description": "Calendar integration status", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Integrations"}}}}
		if method == "delete" {
			status = "204"
			success = map[string]any{"description": "Disconnected; existing remote events remain"}
		}
		responses := map[string]any{status: success}
		for _, code := range []string{"401", "403", "409", "502", "503", "504"} {
			responses[code] = map[string]any{"description": "Integration error", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}}
		}
		op := map[string]any{"tags": []string{"Integrations"}, "responses": responses}
		if method == "post" {
			op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false}}}}
		}
		paths[path] = map[string]any{method: op}
	}
	start := paths["/api/auth/yandex/start"].(map[string]any)["get"].(map[string]any)
	start["description"] = "Browser-bound OAuth with PKCE. calendar=1 requires an active session and requests calendar:all; normal login requests only the profile. The existing callback URI is reused."
	start["parameters"] = []any{map[string]any{"name": "calendar", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"1"}}}, map[string]any{"name": "link", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"1"}}}}
	start["parameters"] = append(start["parameters"].([]any), map[string]any{"name": "source", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"onboarding", "integrations"}}, "description": "Stored in one-use state; onboarding success completes onboarding and redirects to chat. No arbitrary return URL."})
}
