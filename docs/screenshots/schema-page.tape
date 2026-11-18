# An Avro schema in the schema registry.
Source docs/screenshots/setup.tape

# Topics -> Consumer Groups -> Schema Registry
Ctrl+L
Sleep 500ms
Ctrl+L
Wait+Screen@20s /PaymentProcessed/
Type "/"
Type "PaymentProcessed"
Enter
Sleep 500ms
Enter
Wait+Screen@20s /"namespace"/
Show
Sleep 500ms
Screenshot docs/screenshots/schema-page.png
Sleep 100ms
