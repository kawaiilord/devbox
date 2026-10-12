import 'package:flutter/material.dart';

/// Section selection replaces content immediately, as in the admin console.
/// Changing its key also disposes any detail routes belonging to the old section.
class WorkspacePage extends Page<void> {
  const WorkspacePage({super.key, required this.child});
  final Widget child;

  @override
  Route<void> createRoute(BuildContext context) => _WorkspaceRoute(this);
}

class _WorkspaceRoute extends PageRoute<void> {
  _WorkspaceRoute(WorkspacePage page) : super(settings: page);

  @override
  Duration get transitionDuration => Duration.zero;
  @override
  Duration get reverseTransitionDuration => Duration.zero;
  @override
  Color? get barrierColor => null;
  @override
  String? get barrierLabel => null;
  @override
  bool get maintainState => true;
  @override
  Widget buildPage(
    BuildContext context,
    Animation<double> animation,
    Animation<double> secondaryAnimation,
  ) => (settings as WorkspacePage).child;

  @override
  Widget buildTransitions(
    BuildContext context,
    Animation<double> animation,
    Animation<double> secondaryAnimation,
    Widget child,
  ) => child;
}

class WorkspaceObserver extends NavigatorObserver {
  WorkspaceObserver(this.onChanged);
  final VoidCallback onChanged;

  @override
  void didChangeTop(Route<dynamic> topRoute, Route<dynamic>? previousTopRoute) {
    // Navigation can be updated while building the shell's page list.
    WidgetsBinding.instance.addPostFrameCallback((_) => onChanged());
  }
}
