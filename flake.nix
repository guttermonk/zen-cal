{
  description = "Zen-Cal - A minimal, interactive terminal-based calendar";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        zen-cal = pkgs.buildGoModule {
          pname = "zen-cal";
          version = "1.0.0";

          src = ./src;

          vendorHash = "sha256-YeIUXCClNZzjLLsth8hvhTfi+Oes4bwHubRW4lsBOKI=";

          meta = with pkgs.lib; {
            description = "A minimal, interactive terminal-based calendar with event management";
            homepage = "https://github.com/beaterblank/zen-cal";
            license = licenses.mit;
            maintainers = [ ];
            platforms = platforms.linux ++ platforms.darwin;
            mainProgram = "zen-cal";
          };
        };
      in
      {
        packages = {
          default = zen-cal;
          zen-cal = zen-cal;
        };

        apps = {
          default = {
            type = "app";
            program = "${zen-cal}/bin/zen-cal";
          };
          zen-cal = {
            type = "app";
            program = "${zen-cal}/bin/zen-cal";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            go-tools
          ];
        };
      }
    ) // {
      # Home Manager module
      homeManagerModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.programs.zen-cal;
          tomlFormat = pkgs.formats.toml { };
        in
        {
          options.programs.zen-cal = {
            enable = lib.mkEnableOption "zen-cal terminal calendar";

            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.stdenv.hostPlatform.system}.zen-cal;
              description = "The zen-cal package to use.";
            };

            settings = lib.mkOption {
              type = lib.types.attrsOf lib.types.str;
              default = { };
              example = lib.literalExpression ''
                {
                  today = "#f38ba8";
                  today_text = "#cdd6f4";
                  headings = "#cba6f7";
                  text = "#cdd6f4";
                  weekends = "#f9e2af";
                  max_events = "5";
                  key_prev_month = "h, left";
                  key_next_month = "l, right";
                }
              '';
              description = "Configuration options for zen-cal.";
            };

            events = lib.mkOption {
              type = lib.types.listOf (lib.types.submodule {
                options = {
                  date = lib.mkOption {
                    type = lib.types.str;
                    description = "Event date in YYYY-MM-DD format.";
                    example = "2025-01-15";
                  };
                  title = lib.mkOption {
                    type = lib.types.str;
                    description = "Event title.";
                    example = "Team Meeting";
                  };
                  description = lib.mkOption {
                    type = lib.types.str;
                    default = "";
                    description = "Optional event description.";
                    example = "Weekly sync with the team";
                  };
                  calendar = lib.mkOption {
                    type = lib.types.str;
                    default = "";
                    description = "Calendar name this event belongs to.";
                    example = "work";
                  };
                  time = lib.mkOption {
                    type = lib.types.str;
                    default = "all-day";
                    description = "Event time in HH:MM format, or 'all-day' for full-day events.";
                    example = "09:00";
                  };
                  freeBusy = lib.mkOption {
                    type = lib.types.enum [ "busy" "free" ];
                    default = "busy";
                    description = "Whether the event marks time as busy or free.";
                    example = "busy";
                  };
                };
              });
              default = [ ];
              description = "List of calendar events.";
              example = lib.literalExpression ''
                [
                  { date = "2025-01-15"; time = "09:00"; title = "Team Meeting"; description = "Weekly sync"; calendar = "work"; freeBusy = "busy"; }
                  { date = "2025-02-14"; time = "all-day"; title = "Valentine's Day"; calendar = "family"; freeBusy = "free"; }
                ]
              '';
            };

            calendars = lib.mkOption {
              type = lib.types.attrsOf lib.types.str;
              default = { };
              example = lib.literalExpression ''
                {
                  personal = "#f38ba8";
                  work = "#89b4fa";
                  family = "#a6e3a1";
                }
              '';
              description = "Calendar definitions with their colors.";
            };

            showLegend = lib.mkOption {
              type = lib.types.bool;
              default = true;
              description = "Whether to show the color legend on the calendar.";
            };

            showHolidays = lib.mkOption {
              type = lib.types.bool;
              default = false;
              description = "Whether to show US federal holidays on the calendar.";
            };

            showWeekNumbers = lib.mkOption {
              type = lib.types.bool;
              default = true;
              description = "Whether to show ISO week numbers in the leftmost column.";
            };

            default_calendar = lib.mkOption {
              type = lib.types.str;
              default = "";
              description = "Default calendar for new events (use folder name, not display name).";
              example = "personal";
            };

            eventIndicatorDays = lib.mkOption {
              type = lib.types.int;
              default = 0;
              description = "Days before an event to show the Waybar indicator (0 = day of only).";
              example = 1;
            };
          };

          config = lib.mkIf cfg.enable {
            home.packages = [ cfg.package ];

            xdg.configFile."zen-cal/zen-cal.conf" = lib.mkIf (cfg.settings != { } || cfg.calendars != { } || !cfg.showLegend || cfg.showHolidays || !cfg.showWeekNumbers || cfg.default_calendar != "" || cfg.eventIndicatorDays != 0) {
              text = lib.concatStringsSep "\n" (
                (lib.mapAttrsToList (name: value: "${name} = ${value}") cfg.settings)
                ++ [ "show_legend = ${if cfg.showLegend then "true" else "false"}" ]
                ++ [ "show_holidays = ${if cfg.showHolidays then "true" else "false"}" ]
                ++ [ "show_week_numbers = ${if cfg.showWeekNumbers then "true" else "false"}" ]
                ++ [ "event_indicator_days = ${toString cfg.eventIndicatorDays}" ]
                ++ (lib.optional (cfg.default_calendar != "") "default_calendar = ${cfg.default_calendar}")
                ++ (lib.mapAttrsToList (name: color: "calendar.${name} = ${color}") cfg.calendars)
              );
            };

            xdg.configFile."zen-cal/events.conf" = lib.mkIf (cfg.events != [ ]) {
              text = lib.concatStringsSep "\n" (
                [ "# Zen-Cal Events" "# Format: YYYY-MM-DD | Time | Title | Description | Calendar | FreeBusy" "" ] ++
                (map (event:
                  "${event.date} | ${event.time} | ${event.title} | ${event.description} | ${event.calendar} | ${event.freeBusy}"
                ) cfg.events)
              );
            };
          };
        };

      # NixOS module (for system-wide installation)
      nixosModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.programs.zen-cal;
        in
        {
          options.programs.zen-cal = {
            enable = lib.mkEnableOption "zen-cal terminal calendar";

            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.stdenv.hostPlatform.system}.zen-cal;
              description = "The zen-cal package to use.";
            };
          };

          config = lib.mkIf cfg.enable {
            environment.systemPackages = [ cfg.package ];
          };
        };

      # Overlay for use in other flakes
      overlays.default = final: prev: {
        zen-cal = self.packages.${prev.system}.zen-cal;
      };
    };
}
