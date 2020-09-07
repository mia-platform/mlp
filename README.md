# Deploy

This repository contains the script for deploying to a k8s cluster. The script is responsible of interpolating the environment variables.

## Run

Usage: go run main.go -prefix `primary_prefix` -fallback-prefix `fallback prefix` -file `file_to_interpolate`

The script will search for all the strings between `{{}}` and will replace it with the environment variabile content. To find the variable the script will use a `-prefix` to add in order to get the variable name and it will use the `-fallback-prefix` whenever the primary one has no environment variables associated to it.
