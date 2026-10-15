package api

func notificationsOpenAPI(paths, schemas map[string]any) {
	str := func() map[string]any { return map[string]any{"type": "string"} }
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	jsonContent := func(schema map[string]any) map[string]any {
		return map[string]any{"application/json": map[string]any{"schema": schema}}
	}
	schemas["PushSubscribe"] = object(map[string]any{"endpoint": map[string]any{"type": "string", "format": "uri", "maxLength": 4096}, "platform": map[string]any{"type": "string", "enum": []string{"iPhone", "iPad", "Android", "Mac", "Windows", "Linux", "Этот браузер"}}, "keys": object(map[string]any{"auth": str(), "p256dh": str()}, "auth", "p256dh")}, "endpoint", "keys", "platform")
	properties := map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "platform": str(), "failure_count": map[string]any{"type": "integer"}}
	for _, field := range []string{"created_at", "updated_at", "last_success_at", "last_failure_at", "disabled_at"} {
		properties[field] = map[string]any{"type": "string", "format": "date-time", "nullable": field != "created_at" && field != "updated_at"}
	}
	schemas["PushDevice"] = object(properties, "id", "platform")
	ref := map[string]any{"$ref": "#/components/schemas/PushDevice"}
	for _, entry := range []struct {
		path, method, status, description string
		request, response                 map[string]any
	}{
		{"/api/notifications/push/config", "get", "200", "Public key only; enabled=false when not configured", nil, object(map[string]any{"enabled": map[string]any{"type": "boolean"}, "public_key": str()}, "enabled", "public_key")},
		{"/api/notifications/push/subscriptions", "get", "200", "Only the current user's devices; endpoint and encryption keys are never returned", nil, object(map[string]any{"subscriptions": map[string]any{"type": "array", "items": ref}}, "subscriptions")},
		{"/api/notifications/push/subscriptions", "post", "201", "Idempotent by endpoint; never reassigns another user's device", map[string]any{"$ref": "#/components/schemas/PushSubscribe"}, object(map[string]any{"subscription": ref}, "subscription")},
		{"/api/notifications/push/subscriptions/{id}", "delete", "204", "Disable an owned device and cancel pending deliveries", nil, nil},
		{"/api/notifications/push/test", "post", "202", "Queue a real push to an owned active device; one test per user per minute across instances", object(map[string]any{"subscription_id": map[string]any{"type": "string", "format": "uuid"}}, "subscription_id"), object(map[string]any{"queued": map[string]any{"type": "boolean"}}, "queued")},
	} {
		success := map[string]any{"description": entry.description}
		if entry.response != nil {
			success["content"] = jsonContent(entry.response)
		}
		responses := map[string]any{entry.status: success}
		for _, code := range []string{"401", "403", "404", "409", "422", "429", "503"} {
			responses[code] = map[string]any{"description": "Request rejected", "content": jsonContent(map[string]any{"$ref": "#/components/schemas/Error"})}
		}
		op := map[string]any{"tags": []string{"Notifications"}, "description": entry.description, "responses": responses}
		if entry.request != nil {
			op["requestBody"] = map[string]any{"required": true, "content": jsonContent(entry.request)}
		}
		if entry.method == "delete" {
			op["parameters"] = []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}}}
		}
		if paths[entry.path] == nil {
			paths[entry.path] = map[string]any{}
		}
		paths[entry.path].(map[string]any)[entry.method] = op
	}
	paths["/api/reminders/due"].(map[string]any)["get"].(map[string]any)["description"] = "Read recent persisted reminder snapshots; does not schedule, deliver or advance recurrence."
}
