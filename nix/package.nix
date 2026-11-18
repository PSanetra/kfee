{
  lib,
  buildGoModule,
  makeWrapper,
  protobuf,
  version ? "dev",
  # protoc is used to decode protobuf records with .proto files
  withProtoc ? true,
}:

buildGoModule {
  pname = "kfee";
  inherit version;

  src = lib.fileset.toSource {
    root = ./..;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      (lib.fileset.fileFilter (file: file.hasExt "go") ./..)
    ];
  };

  # update after changing go.mod: set to lib.fakeHash, run nix build and use the reported hash
  vendorHash = "sha256-SzBQkX4jj3u4Mg1PphY+Yn1NV8Q3rsLqkpsay2dnDMw=";

  subPackages = [ "cmd/kfee" ];
  tags = [ "prd" ];
  env.CGO_ENABLED = 0;
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  nativeBuildInputs = lib.optional withProtoc makeWrapper;
  # appended to the PATH, a protoc installed by the user takes precedence
  postInstall = lib.optionalString withProtoc ''
    wrapProgram $out/bin/kfee --suffix PATH : ${lib.makeBinPath [ protobuf ]}
  '';

  meta = {
    description = "Kafka terminal client, a fork of ktea";
    homepage = "https://github.com/PSanetra/kfee";
    license = lib.licenses.asl20;
    mainProgram = "kfee";
  };
}
