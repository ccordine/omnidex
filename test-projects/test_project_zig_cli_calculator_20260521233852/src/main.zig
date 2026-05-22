const std = @import("std");

const Operation = enum { add, subtract, multiply, divide, power, sqrt };

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();
    const args = try std.process.argsAlloc(allocator);
    defer std.process.argsFree(allocator, args);

    if (args.len == 1 or std.mem.eql(u8, args[1], "--help") or std.mem.eql(u8, args[1], "-h")) {
        try printHelp();
        return;
    }
    if (args.len < 3) {
        try failUsage("missing operation or operand");
    }

    const op = parseOperation(args[1]) orelse try failUsage("unknown operation");
    const result = switch (op) {
        .sqrt => blk: {
            const value = try parseNumber(args[2]);
            if (value < 0) return error.NegativeSquareRoot;
            break :blk std.math.sqrt(value);
        },
        else => blk: {
            if (args.len < 4) try failUsage("binary operation requires two operands");
            const left = try parseNumber(args[2]);
            const right = try parseNumber(args[3]);
            break :blk try calculate(op, left, right);
        },
    };
    try std.io.getStdOut().writer().print("{d}\n", .{result});
}

fn printHelp() !void {
    try std.io.getStdOut().writer().writeAll(
        "zig-cli-calculator\n" ++
        "Usage:\n" ++
        "  zig build run -- <add|sub|mul|div|pow> <left> <right>\n" ++
        "  zig build run -- sqrt <value>\n" ++
        "Examples:\n" ++
        "  zig build run -- add 2 3\n" ++
        "  zig build run -- pow 2 8\n"
    );
}

fn failUsage(message: []const u8) !noreturn {
    try std.io.getStdErr().writer().print("error: {s}\nRun with --help for usage.\n", .{message});
    return error.InvalidUsage;
}

fn parseOperation(raw: []const u8) ?Operation {
    if (std.mem.eql(u8, raw, "add") or std.mem.eql(u8, raw, "+")) return .add;
    if (std.mem.eql(u8, raw, "sub") or std.mem.eql(u8, raw, "subtract") or std.mem.eql(u8, raw, "-")) return .subtract;
    if (std.mem.eql(u8, raw, "mul") or std.mem.eql(u8, raw, "multiply") or std.mem.eql(u8, raw, "*")) return .multiply;
    if (std.mem.eql(u8, raw, "div") or std.mem.eql(u8, raw, "divide") or std.mem.eql(u8, raw, "/")) return .divide;
    if (std.mem.eql(u8, raw, "pow") or std.mem.eql(u8, raw, "power")) return .power;
    if (std.mem.eql(u8, raw, "sqrt")) return .sqrt;
    return null;
}

fn parseNumber(raw: []const u8) !f64 {
    return std.fmt.parseFloat(f64, raw);
}

fn calculate(op: Operation, left: f64, right: f64) !f64 {
    return switch (op) {
        .add => left + right,
        .subtract => left - right,
        .multiply => left * right,
        .divide => if (right == 0) error.DivideByZero else left / right,
        .power => std.math.pow(f64, left, right),
        .sqrt => unreachable,
    };
}

test "calculator arithmetic" {
    try std.testing.expectEqual(@as(f64, 5), try calculate(.add, 2, 3));
    try std.testing.expectEqual(@as(f64, -1), try calculate(.subtract, 2, 3));
    try std.testing.expectEqual(@as(f64, 6), try calculate(.multiply, 2, 3));
    try std.testing.expectEqual(@as(f64, 4), try calculate(.divide, 8, 2));
    try std.testing.expectEqual(@as(f64, 8), try calculate(.power, 2, 3));
}
