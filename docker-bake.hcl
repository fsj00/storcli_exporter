variable "REGISTRY" {
  default = "git.hener.app/hener-oss"
}

variable "COMMIT" {}

tags = [
  "latest",
  COMMIT,
]

target "default" {
  platforms = ["linux/amd64"]
  dockerfile = "Dockerfile"

  tags = [
    for tag in tags:
    "${REGISTRY}/storcli-exporter:${tag}"
  ]
}
