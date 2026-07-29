{
  pkgs ? import <nixpkgs> { },
}: pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    gopls

    # pkg-config
    # libGL
    libGL.dev
    libGL
    zig

    libX11.dev
    #wayland
    gtk3
    gtk3.dev
  ];
  nativeBuildInputs = with pkgs; [
      pkg-config
  ];

  shellHook = ''
    export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath [ pkgs.libGL pkgs.libGL.dev pkgs.gtk3 pkgs.gtk3.dev ]}:$LD_LIBRARY_PATH"
    export XDG_DATA_DIRS="$XDG_DATA_DIRS:${pkgs.gtk3}/share/gsettings-schemas/${pkgs.gtk3.name}"
  '';
}
