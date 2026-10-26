resource "vaultwarden_item_secure_note" "server" {
  name = "Web server"
}

# Attach a file. Changing the file on disk replaces the attachment
resource "vaultwarden_item_attachment" "certificate" {
  item_id = vaultwarden_item_secure_note.server.id
  file    = "${path.module}/certs/server.pem"
}

# Attach generated text, with an explicit file name
resource "vaultwarden_item_attachment" "config" {
  item_id   = vaultwarden_item_secure_note.server.id
  file_name = "config.json"
  content   = jsonencode({ listen = ":443" })
}

# Attach binary content
resource "vaultwarden_item_attachment" "keystore" {
  item_id        = vaultwarden_item_secure_note.server.id
  file_name      = "keystore.p12"
  content_base64 = filebase64("${path.module}/certs/keystore.p12")
}
