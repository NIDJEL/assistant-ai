package api

func socialOpenAPI(paths, schemas map[string]any) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/components/schemas/" + name} }
	content := func(schema any) map[string]any {
		return map[string]any{"application/json": map[string]any{"schema": schema}}
	}
	schemas["PlanningSlot"] = map[string]any{"type": "object", "required": []string{"start", "end"}, "properties": map[string]any{"start": map[string]any{"type": "string", "format": "date-time"}, "end": map[string]any{"type": "string", "format": "date-time"}}, "description": "Absolute RFC3339 instants. Future start, 5–1440 minutes, within 90 days."}
	schemas["SocialOperation"] = map[string]any{"type": "object", "required": []string{"action", "request_id"}, "additionalProperties": false, "properties": map[string]any{
		"action":     map[string]any{"type": "string", "enum": []string{"request", "accept_request", "decline_request", "cancel_request", "remove_friend", "tags", "settings", "create_meeting", "invite", "set_participants", "respond", "propose", "resolve_proposal", "cancel_meeting", "cancel_invitation"}},
		"request_id": map[string]any{"type": "string", "format": "uuid"}, "id": map[string]any{"type": "string", "format": "uuid"}, "code": map[string]any{"type": "string"}, "user_id": map[string]any{"type": "string", "format": "uuid"}, "tags": map[string]any{"type": "array", "maxItems": 12, "items": map[string]any{"type": "string", "maxLength": 40}}, "event_id": map[string]any{"type": "integer"}, "event": ref("EventCreate"), "friend_ids": map[string]any{"type": "array", "maxItems": 30, "items": map[string]any{"type": "string", "format": "uuid"}}, "status": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer"}, "slots": map[string]any{"type": "array", "maxItems": 5, "items": ref("PlanningSlot")}, "slot_index": map[string]any{"type": "integer", "minimum": 0}, "share_email": map[string]any{"type": "boolean"}, "timezone": map[string]any{"type": "string"}, "start_hour": map[string]any{"type": "integer", "minimum": 0, "maximum": 23}, "end_hour": map[string]any{"type": "integer", "minimum": 1, "maximum": 24},
	}, "description": "User-scoped, transactional and idempotent. Reuse request_id only for an identical retry. IDs identify a request/proposal; event_id identifies the existing event; version is required for existing shared meetings. RSVP always updates the authenticated participant. Time changes reset responses to pending."}
	for path, method := range map[string]string{"/api/social/people": "get", "/api/social/meetings": "get", "/api/social/operations": "post", "/api/social/availability": "post"} {
		responses := map[string]any{"200": map[string]any{"description": "Authorized result"}}
		for _, status := range []string{"401", "403", "404", "409", "422", "429", "503"} {
			responses[status] = map[string]any{"description": "Authorization, stale version, validation or rate limit error", "content": content(ref("Error"))}
		}
		operation := map[string]any{"tags": []string{"Friends and scheduling"}, "responses": responses}
		if path == "/api/social/operations" {
			operation["requestBody"] = map[string]any{"required": true, "content": content(ref("SocialOperation"))}
		}
		if path == "/api/social/availability" {
			operation["description"] = "Only own/friend or authorized meeting availability. Returns free/busy/unknown and at most six common candidate slots over 14 days, never personal event details. External outages set complete=false. Explicit final selection is required."
			operation["requestBody"] = map[string]any{"required": true, "content": content(map[string]any{"type": "object", "required": []string{"slot"}, "properties": map[string]any{"slot": ref("PlanningSlot"), "friend_ids": map[string]any{"type": "array", "maxItems": 30, "items": map[string]any{"type": "string", "format": "uuid"}}, "event_id": map[string]any{"type": "integer"}}})}
		}
		paths[path] = map[string]any{method: operation}
	}
}
