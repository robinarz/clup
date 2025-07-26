# Define the output directory for the binary.
BIN_DIR := bin

# Define the name of the binary.
BINARY_NAME := clup

# Define the path to the main package. Since main.go is in the root, this is just ".".
CMD_PATH := .

# The default target, which is an alias for 'build'.
.PHONY: all
all: build

# Build the clup application.
.PHONY: build
build: $(BIN_DIR)/$(BINARY_NAME)

$(BIN_DIR)/$(BINARY_NAME):
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	@go build -o $@ $(CMD_PATH)
	@echo "✅ $(BINARY_NAME) built successfully in $(BIN_DIR)/"

# Clean the build artifacts by removing the BIN_DIR.
.PHONY: clean
clean:
	@echo "Cleaning up..."
	@rm -rf $(BIN_DIR)
	@echo "Cleanup complete."

# Install the binary to the user's local bin directory.
# This makes the 'clup' command available globally for the current user.
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to $(HOME)/.local/bin..."
	@mkdir -p $(HOME)/.local/bin
	@cp $(BIN_DIR)/$(BINARY_NAME) $(HOME)/.local/bin/
	@echo "✅ $(BINARY_NAME) installed successfully. Make sure $(HOME)/.local/bin is in your PATH."

