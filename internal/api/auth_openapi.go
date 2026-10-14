package api

func authOpenAPI(paths, schemas map[string]any) {
	text := func() map[string]any { return map[string]any{"type": "string"} }
	schemas["User"] = map[string]any{"type": "object", "required": []string{"id", "name", "providers", "onboarding_completed"}, "properties": map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "name": text(), "email": text(), "avatar_url": text(), "providers": map[string]any{"type": "array", "items": text()}, "onboarding_completed": map[string]any{"type": "boolean"}}}
	schemas["AuthResponse"] = map[string]any{"type": "object", "properties": map[string]any{"user": map[string]any{"$ref": "#/components/schemas/User"}}}
	schemas["LoginRequest"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"email", "password"}, "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}, "password": map[string]any{"type": "string", "format": "password", "writeOnly": true, "maxLength": 1024}}}
	schemas["RegisterRequest"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "email", "password", "password_confirmation"}, "properties": map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 100}, "email": map[string]any{"type": "string", "format": "email"}, "password": map[string]any{"type": "string", "format": "password", "writeOnly": true, "minLength": 10, "maxLength": 1024}, "password_confirmation": map[string]any{"type": "string", "format": "password", "writeOnly": true}}}
	for path, methods := range map[string][]string{"/api/auth/register": {"post"}, "/api/auth/login": {"post"}, "/api/auth/logout": {"post"}, "/api/auth/me": {"get", "patch"}, "/api/auth/onboarding": {"post"}} {
		ops := map[string]any{}
		for _, method := range methods {
			status := "200"
			if path == "/api/auth/register" {
				status = "201"
			}
			if path == "/api/auth/logout" {
				status = "204"
			}
			success := map[string]any{"description": "Success"}
			if status != "204" {
				success["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/AuthResponse"}}}
			}
			responses := map[string]any{status: success}
			for _, code := range []string{"401", "403", "409", "422", "429"} {
				responses[code] = map[string]any{"description": "Inline authentication error", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}}
			}
			op := map[string]any{"responses": responses, "tags": []string{"Authentication"}}
			if path == "/api/auth/login" || path == "/api/auth/register" || path == "/api/auth/logout" {
				op["security"] = []any{}
			}
			schema := ""
			if path == "/api/auth/login" {
				schema = "LoginRequest"
			}
			if path == "/api/auth/register" {
				schema = "RegisterRequest"
			}
			if schema != "" {
				op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + schema}}}}
			}
			if method == "patch" {
				op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false, "minProperties": 1, "properties": map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 100}, "avatar_url": map[string]any{"type": "string", "description": "JPEG/PNG data URL up to 256 x 256 and 40 KiB, or an empty string to remove the photo"}}}}}}
			}
			if path == "/api/auth/onboarding" || path == "/api/auth/logout" {
				op["requestBody"] = map[string]any{"required": true, "description": "Send an empty JSON object for same-origin CSRF protection", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}}
			}
			ops[method] = op
		}
		paths[path] = ops
	}
	for _, provider := range []string{"yandex", "vk"} {
		for _, suffix := range []string{"start", "callback"} {
			paths["/api/auth/"+provider+"/"+suffix] = map[string]any{"get": map[string]any{"security": []any{}, "tags": []string{"Authentication"}, "description": "Authorization code with browser-bound state and PKCE. Start accepts link=1 only for an active user; callback consumes one-use state.", "responses": map[string]any{"303": map[string]any{"description": "Redirect to provider, app or inline auth error"}, "429": map[string]any{"description": "Rate limited"}}}}
		}
	}
}
