{
  pkgs ? import <nixpkgs> { },
}: pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    gopls

    pkg-config
    libGL

    xorg.libX11
    wayland
  ];

  shellHook = ''
    export LD_LIBRARY_PATH="$PWD/modules/evdi/library:${pkgs.lib.makeLibraryPath [ pkgs.libGL ]}:$LD_LIBRARY_PATH"
  '';
}
