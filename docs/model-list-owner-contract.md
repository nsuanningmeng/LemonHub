# Model list ownership metadata

The public OpenAI-compatible `GET /v1/models` response uses `owned_by` to identify the provider or channel **type**, not an individual configured channel. For example, multiple Advanced Custom channels can legitimately return `"owned_by": "advanced_custom"`.

Administrators can distinguish individual instances in channel management using their private channel names and configuration. Those names, credentials, and upstream URLs are not public model metadata and are not copied into `owned_by`. A model can also be served by more than one authorized channel; the model list does not promise that one instance will handle every request.

This contract preserves the existing model-list wire format and access/group filtering. It does not introduce public per-instance aliases. Such a feature would require an explicitly public field and defined selection and tenant-visibility rules.
