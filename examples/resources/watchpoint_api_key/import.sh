# Import restores every attribute except api_key, which the API returns only once
# at creation and can never return again. Rotate the key after importing if the
# configuration needs the secret in state.
terraform import watchpoint_api_key.example apikey_001
