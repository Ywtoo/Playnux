# Playnux roadmap

## Now · Stabilize the alpha

- Fix issues reported in library imports, game launching, Big Picture, and integrations.
- Test installation, updates, and library migrations on more Linux distributions.
- Reorganize the existing code and clarify the responsibilities of the core, interface, and each integration. Extend regression coverage for existing flows.

## Explore · Metadata options for Playnux

Today, IGDB and SteamGridDB require each user to provide their own credentials in **Settings → Metadata**. I’ll study whether that setup can be simplified and compare the best options for Playnux.

- Compare local improvements with a shared cache or a metadata service exclusively for the Playnux app.
- Evaluate provider coverage, caching terms, attribution, and hosting costs before choosing an approach. Confirm any proposed use of IGDB with the provider.
- Keep personal keys available while evaluating alternatives.

## Later · Plugins and themes

- Define versioned contracts for independent plugins and migrate built-in integrations gradually.
- Document how to install, update, and disable plugins, along with their permissions and error reporting.
- Build an installable theme system with a documented format and support for keyboard and controller navigation.

## After that · Community catalog

- Publish a static site to browse plugins and themes, check version compatibility, and open their source repositories.
- Document how maintainers can add, update, and remove catalog entries.
- Explore a store for community content.

## Funding

Playnux is free and open source. Donations help cover the time spent maintaining it and future hosting costs. See [Support the project](../README.md#support-the-project).
