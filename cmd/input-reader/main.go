package main

import (
	"fmt"
	"log"

	"github.com/eiannone/keyboard"
)

// LineNotification represents a notification with the line read and the line ending character.
type LineNotification struct {
	Line       string // The line read
	LineEnding rune   // The line ending character identified
}

// InputReader reads input from the keyboard and notifies when a line ending is detected.
type InputReader struct {
	lineEndings map[rune]bool         // Map of line ending characters
	outputChan  chan LineNotification // Channel to send notifications
}

// NewInputReader creates a new InputReader with the specified line endings.
func NewInputReader(lineEndings []rune) *InputReader {
	// Create a map for line ending characters
	endings := make(map[rune]bool)
	for _, le := range lineEndings {
		endings[le] = true
	}

	return &InputReader{
		lineEndings: endings,
		outputChan:  make(chan LineNotification),
	}
}

// Start begins reading input from the keyboard.
func (ir *InputReader) Start() {
	go func() {
		// Open the keyboard for reading input
		if err := keyboard.Open(); err != nil {
			log.Fatalf("Error opening keyboard: %v", err)
		}
		defer keyboard.Close() // Close the keyboard when done

		var buffer []rune // Buffer to accumulate characters

		for {
			// Read a character from the keyboard
			char, key, err := keyboard.GetKey()
			if err != nil {
				log.Fatalf("Error reading keyboard: %v", err)
			}

			// Handle Backspace key
			if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
				if len(buffer) > 0 {
					buffer = buffer[:len(buffer)-1] // Remove the last character
					fmt.Print("\b \b")              // Overwrite the last character in the terminal
				}
				continue
			}

			// Check if the key is a line ending (Tab or Enter)
			isLineEnding := false
			var lineEnding rune

			// Check if the character is a line ending (e.g., Tab)
			if ir.lineEndings[char] {
				isLineEnding = true
				lineEnding = char
			}

			// Check if the key is Enter
			if key == keyboard.KeyEnter {
				isLineEnding = true
				lineEnding = '\n' // Use '\n' to represent Enter
			}

			// Check if the key is Tab
			if key == keyboard.KeyTab {
				isLineEnding = true
				lineEnding = '\t' // Use '\t' to represent Tab
			}

			// If it's a line ending, send the notification
			if isLineEnding {
				line := string(buffer) // Convert the buffer to a string
				ir.outputChan <- LineNotification{
					Line:       line,
					LineEnding: lineEnding,
				}
				buffer = buffer[:0] // Reset the buffer
				fmt.Println()       // Move to a new line after notification
				continue
			}

			// Handle Esc or Ctrl+C to exit
			if key == keyboard.KeyEsc || key == keyboard.KeyCtrlC {
				break
			}

			// Add the character to the buffer
			if char != 0 {
				buffer = append(buffer, char)
				fmt.Printf("%c", char) // Print the pressed character
			}
		}

		// Close the channel when exiting the loop
		close(ir.outputChan)
	}()
}

// GetOutputChan returns the channel for receiving notifications.
func (ir *InputReader) GetOutputChan() <-chan LineNotification {
	return ir.outputChan
}

func main() {
	// Define the line ending characters (e.g., Tab and Enter)
	lineEndings := []rune{'\t', '\n'}

	// Create a new InputReader
	reader := NewInputReader(lineEndings)

	// Start reading input
	reader.Start()

	// Read from the channel and handle notifications
	for notification := range reader.GetOutputChan() {
		fmt.Printf("Received line: %s (Line ending: %q)\n", notification.Line, notification.LineEnding)
	}

	fmt.Println("Program terminated.")
}
