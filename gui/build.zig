const std = @import("std");
const go = @import("./go.zig");

pub fn build(b: *std.Build) !void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    //: build step
    const go_build = go.addExecutable(b, .{
        .name = "eurydice",
        .target = target,
        .optimize = optimize,
        .package_path = b.path(""),
    });

    //: run step
    const run_cmd = go_build.addRunStep();
    if (b.args) |args| {
        run_cmd.addArgs(args);
    }
    const run_step = b.step("run", "Run Eurydice");
    run_step.dependOn(&run_cmd.step);

    //: install executable
    go_build.addInstallStep();
}
