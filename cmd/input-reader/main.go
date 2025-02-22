package main

import (
	"fmt"
	"io"
	"log"

	"github.com/eiannone/keyboard"
)

// LineNotification represents a notification with the line read and the line ending key.
type LineNotification struct {
	Line       string       // The line read
	LineEnding keyboard.Key // The key that caused the line to end
}

// CustomIO handles input and output operations.
type CustomIO struct {
	lineEndingKeys map[keyboard.Key]bool // Set of keys that represent line endings
	outputChan     chan LineNotification // Channel to send notifications
}

// NewCustomIO creates a new CustomIO with the specified line ending keys.
func NewCustomIO(lineEndingKeys []keyboard.Key) *CustomIO {
	// Create a set of line ending keys
	endings := make(map[keyboard.Key]bool)
	for _, key := range lineEndingKeys {
		endings[key] = true
	}

	return &CustomIO{
		lineEndingKeys: endings,
		outputChan:     make(chan LineNotification),
	}
}

// Start begins reading input from the keyboard.
func (cio *CustomIO) Start() {
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

			// Check if the key is a line ending key
			if cio.lineEndingKeys[key] {
				line := string(buffer) // Convert the buffer to a string
				cio.outputChan <- LineNotification{
					Line:       line,
					LineEnding: key,
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
		close(cio.outputChan)
	}()
}

// GetLine reads the next line and returns the line and the line ending key.
func (cio *CustomIO) GetLine() (string, keyboard.Key, bool) {
	notification, ok := <-cio.outputChan
	if !ok {
		return "", 0, false // Channel closed
	}
	return notification.Line, notification.LineEnding, true
}

// Write writes data to the standard output.
func (cio *CustomIO) Write(p []byte) (n int, err error) {
	return fmt.Print(string(p)) // Write to standard output
}

// Read reads input from the keyboard and returns it as a byte slice.
func (cio *CustomIO) Read(p []byte) (n int, err error) {
	line, _, ok := cio.GetLine()
	if !ok {
		return 0, io.EOF // No more input
	}
	copy(p, []byte(line))
	return len(line), nil
}

func main() {
	// Define the line ending keys (e.g., Enter, Tab, Arrow Up, Arrow Down)
	lineEndingKeys := []keyboard.Key{
		keyboard.KeyEnter,     // Enter key
		keyboard.KeyTab,       // Tab key
		keyboard.KeyArrowUp,   // Arrow Up key
		keyboard.KeyArrowDown, // Arrow Down key
	}

	// Create a new CustomIO
	customIO := NewCustomIO(lineEndingKeys)

	// Start reading input
	customIO.Start()

	// Use CustomIO for reading and writing
	for {
		line, lineEndingKey, ok := customIO.GetLine()
		if !ok {
			break // Exit if the channel is closed
		}

		if line == "" {
			fmt.Println("Empty line detected. Printing prompt...")
			// Simulate printing a prompt
			fmt.Print("> ")
		} else {
			// Write the received line to standard output
			customIO.Write([]byte(fmt.Sprintf("Transmitting line: %s (Line ending key: %v)\n", line, lineEndingKey)))
			// Simulate transmitting the line
			// m.Transmit(line, lineEndingKey, m.uuid)
		}
	}

	fmt.Println("Program terminated.")
}
