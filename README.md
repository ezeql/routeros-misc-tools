# MikroTik Router Utilities

A command-line interface tool for managing and monitoring MikroTik routers. This application provides an interactive way to view and manage DHCP leases with vendor information lookup capabilities.

## Features

- 🔐 Secure SSH connection to MikroTik routers
- 📋 Interactive DHCP lease viewer with sorting capabilities
- 🏢 Automatic MAC vendor lookup using macvendors.com API
- 💾 Vendor information caching to reduce API calls
- 🔑 Credential management with secure storage
- 📊 Beautiful terminal UI using Charm libraries

### DHCP Lease Viewer

- Use arrow keys to navigate the table
- Press `←` `→` to change sort column
- Press `space` to toggle sort order (ascending/descending)
- Press `q`, `esc`, or `ctrl+c` to exit

## Dependencies

- [github.com/charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [github.com/charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) - Style definitions
- [golang.org/x/crypto/ssh](https://golang.org/x/crypto/ssh) - SSH client implementation
- [golang.org/x/term](https://golang.org/x/term) - Terminal utilities

## Configuration

The application stores two configuration files:

- `credentials.json`: Saves router IP and username (password is never stored)
- `vendor_cache.json`: Caches MAC vendor lookups for 30 days

## Security Notes

- SSH passwords are never stored and must be entered each session
- MAC vendor information is cached locally to respect API rate limits
- Uses SSH for secure router communication

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License
