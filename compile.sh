#!/bin/bash

fyne-cross android  -arch=multiple            -app-id="com.mqtt-shell" -debug -no-cache -icon assets/mqtt-shell-mid-resolution.png  ./cmd/mqtt-shell
fyne-cross linux    -arch=amd64,386,arm,arm64 -app-id="com.mqtt-shell" -debug -no-cache -icon assets/mqtt-shell-mid-resolution.png  ./cmd/mqtt-shell
fyne-cross windows  -arch=amd64,386           -app-id="com.mqtt-shell" -debug -no-cache -icon assets/mqtt-shell-mid-resolution.png  ./cmd/mqtt-shell