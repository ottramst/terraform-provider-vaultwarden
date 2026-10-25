resource "vaultwarden_folder" "example" {
  name = "Infrastructure"
}

# Bitwarden clients show a name with slashes as a nested folder
resource "vaultwarden_folder" "nested" {
  name = "Infrastructure/Databases"
}
