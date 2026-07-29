const std = @import("std");

fn goArch(arch: std.Target.Cpu.Arch) ?[]const u8 {
    return switch (arch) {
        .x86_64 => "amd64",
        .x86 => "386",
        .aarch64 => "arm64",
        .arm => "arm",
        .riscv64,
        .s390x,
        => @tagName(arch),
        else => null,
    };
}

pub fn createCompilerWrapper(
    b: *std.Build,
    name: []const u8,
    compiler: []const u8,
    target: []const u8,
) ![]const u8 {
    const wrapper_dir = try b.cache_root.join(b.allocator, &.{"go", "wrappers"});
    try std.Io.Dir.createDirPath(b.build_root.handle, b.graph.io, wrapper_dir);

    const wrapper_path = try std.Io.Dir.path.join(
        b.allocator,
        &.{ wrapper_dir, name },
    );

    const contents = b.fmt(
        "#!/bin/sh\nexec {s} -target {s} $NIX_LDFLAGS \"$@\"\n",
        .{
        compiler,
        target,
    });

    // const file = try std.fs.createFileAbsolute(wrapper_path, .{
    //     .truncate = true,
    // });
    const file = try std.Io.Dir.createFileAbsolute(b.graph.io, wrapper_path, .{
        .permissions = std.Io.File.Permissions.fromMode(0o755),
    });
    defer file.close(b.graph.io);

    // try file.writeAll(contents);
    try file.writeStreamingAll(b.graph.io, contents);

    // make the file executable
    // try file.chmod(0o755);
    // try file.setPermissions(b.graph.io, );

    const final_path = try b.build_root.join(b.allocator, &.{wrapper_path});
    std.debug.print("{s}\n", .{final_path});
    return final_path;
}

pub fn addExecutable(b: *std.Build, options: BuildStep.Options) *BuildStep {
    return BuildStep.create(b, options);
}

pub fn build(b: *std.Build) void {
    _ = b;
}

/// Runs `go build` with relevant flags
pub const BuildStep = struct {
    step: std.Build.Step,
    generated_bin: ?*std.Build.GeneratedFile,
    opts: Options,

    pub const Options = struct {
        name: []const u8,
        target: std.Build.ResolvedTarget,
        optimize: std.builtin.OptimizeMode,
        package_path: std.Build.LazyPath,
        cgo_enabled: bool = true,
    };

    /// Create a GoBuildStep
    pub fn create(b: *std.Build, options: Options) *BuildStep {
        const self = b.allocator.create(BuildStep) catch unreachable;
        self.* = .{
            .opts = options,
            .generated_bin = null,
            .step = std.Build.Step.init(.{
                .id = .custom,
                .name = "go build",
                .owner = b,
                .makeFn = BuildStep.make,
            }),
        };
        return self;
    }

    pub fn make(step: *std.Build.Step, opts: std.Build.Step.MakeOptions) !void {
        const self: *BuildStep = @fieldParentPtr("step", step);
        const b = step.owner;
        var go_args = std.array_list.Managed([]const u8).init(b.allocator);
        defer go_args.deinit();

        try go_args.append("go");
        try go_args.append("build");

        const output_file = try b.cache_root.join(b.allocator, &.{ "go", self.opts.name });
        try go_args.appendSlice(&.{ "-o", output_file });

        switch (self.opts.optimize) {
            .ReleaseSafe => try go_args.appendSlice(&.{ "-tags", "ReleaseSafe" }),
            .ReleaseFast => try go_args.appendSlice(&.{ "-tags", "ReleaseFast" }),
            .ReleaseSmall => try go_args.appendSlice(&.{ "-tags", "ReleaseSmall" }),
            .Debug => try go_args.appendSlice(&.{ "-tags", "Debug" }),
        }

        var env = try b.graph.environ_map.clone(b.allocator);

        // CGO
        if (self.opts.cgo_enabled) {
            try env.put("CGO_ENABLED", "1");
            // Set zig as the CGO compiler
            const target = self.opts.target;
            const target_triple = target.result.zigTriple(b.allocator) catch unreachable;
            const cc_wrapper = try createCompilerWrapper(
                b,
                "zig-cc",
                "zig cc",
                target_triple,
            );
            try env.put("CC", cc_wrapper);
            const cxx_wrapper = try createCompilerWrapper(
                b,
                "zig-cxx",
                "zig c++",
                target_triple,
            );
            try env.put("CXX", cxx_wrapper);
            try env.put("GOOS", @tagName(target.result.os.tag));
            const go_arch = goArch(target.result.cpu.arch) orelse {
                return self.step.fail(
                    "target architecture {s} is not supported by Go",
                    .{@tagName(target.result.cpu.arch)},
                );
            };

            try env.put("GOARCH", go_arch);

            // Tell the linker we are statically linking
            go_args.appendSlice(&.{ "--ldflags", "-linkmode=external -extldflags=-static" }) catch @panic("OOM");
        } else {
            try env.put("CGO_ENABLED", "0");
        }

        // Output file always needs to be added last
        try go_args.append(self.opts.package_path.getPath(b));

        const cmd = std.mem.join(b.allocator, " ", go_args.items) catch @panic("OOM");
        const node = opts.progress_node.start(cmd, 1);
        defer node.end();

        // run the command
        try self.evalChildProcess(go_args.items, &env);

        if (self.generated_bin == null) {
            const generated_bin = b.allocator.create(std.Build.GeneratedFile) catch unreachable;
            generated_bin.* = .{ .step = step };
            self.generated_bin = generated_bin;
        }
        self.generated_bin.?.path = output_file;
    }

    /// Return the LazyPath of the generated binary
    pub fn getEmittedBin(self: *BuildStep) std.Build.LazyPath {
        if (self.generated_bin) |generated_bin|
            return .{ .generated = .{ .file = generated_bin } };

        const b = self.step.owner;
        const generated_bin = b.allocator.create(std.Build.GeneratedFile) catch unreachable;
        generated_bin.* = .{ .step = &self.step };
        self.generated_bin = generated_bin;
        return .{ .generated = .{ .file = generated_bin } };
    }

    /// Add a run step which depends on the GoBuildStep
    pub fn addRunStep(self: *BuildStep) *std.Build.Step.Run {
        const b = self.step.owner;
        const run_step = std.Build.Step.Run.create(b, b.fmt("run {s}", .{self.opts.name}));
        run_step.step.dependOn(&self.step);
        const bin_file = self.getEmittedBin();
        const arg: std.Build.Step.Run.PrefixedLazyPath = .{ .prefix = "", .lazy_path = bin_file };
        run_step.argv.append(b.allocator, .{ .lazy_path = arg }) catch unreachable;
        return run_step;
    }

    // Add an install step which depends on the GoBuildStep
    pub fn addInstallStep(self: *BuildStep) void {
        const b = self.step.owner;
        const bin_file = self.getEmittedBin();
        const install_step = b.addInstallBinFile(bin_file, self.opts.name);
        install_step.step.dependOn(&self.step);
        b.getInstallStep().dependOn(&install_step.step);
    }

    fn evalChildProcess(self: *BuildStep, argv: []const []const u8, env: *const std.process.Environ.Map) !void {
        const s = &self.step;
        const arena = s.owner.allocator;

        try std.Build.Step.handleChildProcUnsupported(s);
        try std.Build.Step.handleVerbose(s.owner, std.process.Child.Cwd.inherit, argv);

        const result = std.process.run(arena, s.owner.graph.io, .{
            .argv = argv,
            .environ_map = env,
        }) catch |err| return s.fail("unable to spawn {s}: {s}", .{ argv[0], @errorName(err) });

        if (result.stderr.len > 0) {
            try s.result_error_msgs.append(arena, result.stderr);
        }

        if (s.result_failed_command != null) {
            try std.Build.Step.handleChildProcessTerm(s, result.term);
        }
    }
};
