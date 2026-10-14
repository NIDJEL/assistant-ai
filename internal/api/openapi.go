package api

import (
	"io"
	"net/http"
)

func OpenAPI() map[string]any {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/components/schemas/" + name} }
	text := func() map[string]any { return map[string]any{"type": "string"} }
	day := map[string]any{"type": "string", "format": "date", "nullable": true}
	clock := map[string]any{"type": "string", "pattern": `^\d{2}:\d{2}$`, "nullable": true}
	enum := func(values ...string) map[string]any { return map[string]any{"type": "string", "enum": values} }
	eventProperties := map[string]any{
		"kind": enum("task", "meeting", "reminder"), "title": map[string]any{"type": "string", "minLength": 1, "maxLength": 500},
		"description": text(), "date": day, "time": clock, "duration_minutes": map[string]any{"type": "integer", "minimum": 5, "maximum": 1440, "default": 60},
		"location": map[string]any{"type": "string", "maxLength": 500}, "meeting_url": map[string]any{"type": "string", "maxLength": 2048, "description": "HTTP(S) URL or empty string"},
		"status": enum("open", "done"), "priority": enum("normal", "low", "high"), "force": map[string]any{"type": "boolean"},
	}
	schemas := map[string]any{
		"ChatRequest": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"message"}, "properties": map[string]any{
			"message": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000}, "session_id": map[string]any{"type": "string", "format": "uuid"}, "request_id": map[string]any{"type": "string", "format": "uuid"}}},
		"EventCreate": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "title"}, "properties": eventProperties},
		"EventPatch": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"title": eventProperties["title"], "description": text(), "location": eventProperties["location"], "meeting_url": eventProperties["meeting_url"], "date": day, "time": clock, "duration_minutes": eventProperties["duration_minutes"], "status": eventProperties["status"], "priority": eventProperties["priority"], "force": eventProperties["force"]}},
		"Event": map[string]any{"type": "object", "required": []string{"id", "kind", "title", "duration_minutes", "status"}, "properties": map[string]any{
			"id": map[string]any{"type": "integer", "format": "int64"}, "kind": eventProperties["kind"], "title": text(), "description": text(), "location": eventProperties["location"], "meeting_url": eventProperties["meeting_url"], "date": day, "time": clock, "duration_minutes": eventProperties["duration_minutes"], "status": eventProperties["status"], "priority": eventProperties["priority"], "created_at": map[string]any{"type": "string", "format": "date-time"}, "updated_at": map[string]any{"type": "string", "format": "date-time"}, "source_key": map[string]any{"type": "string", "nullable": true}, "external_calendar_id": map[string]any{"type": "string", "nullable": true}, "score": map[string]any{"type": "number"}}},
		"Error": map[string]any{"type": "object", "required": []string{"error", "request_id"}, "properties": map[string]any{
			"error": map[string]any{"type": "object", "required": []string{"code", "message"}, "properties": map[string]any{"code": text(), "message": text()}}, "request_id": text(), "detail": text()}},
	}

	schemas["Recurrence"] = map[string]any{"type": "object", "nullable": true, "additionalProperties": false, "required": []string{"frequency", "interval", "timezone"}, "description": "Calendar rules use local wall time; elapsed minute/hour rules use anchor. Missing monthly dates are skipped. DST gap shifts forward, fold fires once at the first instant.", "properties": map[string]any{"frequency": enum("minute", "hour", "day", "week", "month", "year"), "interval": map[string]any{"type": "integer", "minimum": 1, "maximum": 366}, "weekdays": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 0, "maximum": 6}}, "day_of_month": map[string]any{"type": "integer", "minimum": 1, "maximum": 31}, "month": map[string]any{"type": "integer", "minimum": 1, "maximum": 12}, "local_time": clock, "timezone": map[string]any{"type": "string", "description": "IANA timezone"}, "anchor": map[string]any{"type": "string", "format": "date-time"}}}
	metadata := map[string]any{
		"recurrence":       ref("Recurrence"),
		"tags":             map[string]any{"type": "array", "maxItems": 12, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 50}},
		"subtasks":         map[string]any{"type": "array", "maxItems": 50, "items": map[string]any{"type": "object", "required": []string{"id", "title", "done"}, "properties": map[string]any{"id": map[string]any{"type": "string", "maxLength": 80}, "title": map[string]any{"type": "string", "maxLength": 500}, "done": map[string]any{"type": "boolean"}}}},
		"participants":     map[string]any{"type": "array", "maxItems": 50, "items": map[string]any{"type": "object", "required": []string{"name"}, "properties": map[string]any{"name": map[string]any{"type": "string", "maxLength": 100}, "email": map[string]any{"type": "string", "maxLength": 254}}}},
		"reminder_enabled": map[string]any{"type": "boolean", "default": true},
		"reminder_minutes": map[string]any{"type": "integer", "minimum": 0, "maximum": 10080, "description": "Minutes before the event to notify; 0 means at the event time. Enable/disable with reminder_enabled."},
	}
	for name, value := range metadata {
		eventProperties[name] = value
		for _, schema := range []string{"EventPatch", "Event"} {
			schemas[schema].(map[string]any)["properties"].(map[string]any)[name] = value
		}
	}
	schemas["Event"].(map[string]any)["properties"].(map[string]any)["deleted_at"] = map[string]any{"type": "string", "format": "date-time", "description": "Present only for deleted events"}
	schemas["Event"].(map[string]any)["properties"].(map[string]any)["start_second"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 59, "readOnly": true, "description": "Optional seconds component for relative commands; omitted means 0."}
	for _, field := range []string{"occurrence_at", "last_notification_at"} {
		schemas["Event"].(map[string]any)["properties"].(map[string]any)[field] = map[string]any{"type": "string", "format": "date-time", "readOnly": true}
	}
	content := func(schema map[string]any) map[string]any {
		return map[string]any{"application/json": map[string]any{"schema": schema}}
	}
	paths := map[string]any{}
	for path, methods := range map[string][]string{"/api/preferences/{profile}": {"get", "put"}, "/health": {"get"}, "/api/chat": {"post"}, "/api/history/{session_id}": {"get"}, "/api/events": {"get", "post"}, "/api/events/{id}": {"get", "patch", "delete"}, "/api/events/{id}/restore": {"post"}, "/api/tasks": {"get"}, "/api/meetings": {"get"}, "/api/reminders": {"get"}, "/api/reminders/due": {"get"}, "/api/import-demo": {"post"}} {
		ops := map[string]any{}
		for _, method := range methods {
			status := "200"
			if method == "post" && path == "/api/events" {
				status = "201"
			}
			if method == "delete" {
				status = "204"
			}
			responseSchema := map[string]any{"type": "object"}
			if path == "/api/events/{id}" || path == "/api/events/{id}/restore" || (path == "/api/events" && method == "post") {
				responseSchema = ref("Event")
			}
			if method == "get" && (path == "/api/events" || path == "/api/tasks" || path == "/api/meetings" || path == "/api/reminders") {
				responseSchema = map[string]any{"type": "object", "required": []string{"events", "total", "offset", "limit"}, "properties": map[string]any{"events": map[string]any{"type": "array", "items": ref("Event")}, "total": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "limit": map[string]any{"type": "integer"}}}
			}
			responses := map[string]any{status: map[string]any{"description": "Успешный результат", "content": content(responseSchema)}}
			if status == "204" {
				responses[status] = map[string]any{"description": "Запись удалена"}
			}
			for _, code := range []string{"400", "404", "405", "409", "413", "422", "500", "504"} {
				responses[code] = map[string]any{"description": "Ошибка: error.code/message", "content": content(ref("Error"))}
			}
			op := map[string]any{"responses": responses}
			if path == "/api/chat" {
				op["requestBody"] = map[string]any{"required": true, "content": content(ref("ChatRequest"))}
			}
			if method == "post" && path == "/api/events" {
				op["requestBody"] = map[string]any{"required": true, "content": content(ref("EventCreate"))}
			}
			if path == "/api/events/{id}/restore" {
				op["requestBody"] = map[string]any{"required": true, "content": content(map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"force": map[string]any{"type": "boolean", "default": false}}})}
			}
			if method == "patch" {
				op["requestBody"] = map[string]any{"required": true, "content": content(ref("EventPatch"))}
			}
			parameters := []any{}
			if path == "/api/preferences/{profile}" {
				parameters = append(parameters, map[string]any{"name": "profile", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}})
				if method == "put" {
					op["requestBody"] = map[string]any{"required": true, "content": content(map[string]any{"type": "object", "description": "Personal browser profile preferences: panel visibility, per-page widget order, hidden types and settings. Maximum request size 64 KiB."})}
				}
			}

			if path == "/api/events/{id}" || path == "/api/events/{id}/restore" {
				parameters = append(parameters, map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer", "format": "int64", "minimum": 1}})
			}
			if path == "/api/history/{session_id}" {
				parameters = append(parameters, map[string]any{"name": "session_id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}})
			}
			if method == "get" && (path == "/api/events" || path == "/api/tasks" || path == "/api/meetings" || path == "/api/reminders") {
				for name, schema := range map[string]any{"start": day, "end": day, "kind": eventProperties["kind"], "status": eventProperties["status"], "q": map[string]any{"type": "string", "maxLength": 200}, "deleted": map[string]any{"type": "boolean", "default": false, "description": "Only deleted events, available for restoration"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 100}, "offset": map[string]any{"type": "integer", "minimum": 0, "default": 0}} {
					parameters = append(parameters, map[string]any{"name": name, "in": "query", "schema": schema})
				}
			}
			if len(parameters) > 0 {
				op["parameters"] = parameters
			}
			ops[method] = op
		}
		paths[path] = ops
	}
	authOpenAPI(paths, schemas)
	integrationsOpenAPI(paths, schemas)
	paths["/health"].(map[string]any)["get"].(map[string]any)["security"] = []any{}
	return map[string]any{"openapi": "3.0.3", "info": map[string]string{"title": "Ассистент Гена — multi-user API", "version": "0.4.0"}, "paths": paths, "security": []map[string]any{{"sessionCookie": []string{}}}, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"sessionCookie": map[string]any{"type": "apiKey", "in": "cookie", "name": sessionCookie}}}}
}
func (a *API) docs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html lang="ru"><head><meta charset="utf-8"><title>Ассистент Гена — Go API</title><link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script><script>SwaggerUIBundle({url:'/openapi.json',dom_id:'#swagger-ui'});</script></body></html>`)
}
