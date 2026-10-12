import 'package:flutter/material.dart';

import '../design.dart';

class WorkspaceSectionInfo extends InheritedWidget {
  const WorkspaceSectionInfo({
    super.key,
    required this.eyebrow,
    required this.description,
    required super.child,
  });

  final String eyebrow, description;

  @override
  bool updateShouldNotify(WorkspaceSectionInfo oldWidget) =>
      eyebrow != oldWidget.eyebrow || description != oldWidget.description;
}

/// A section owns its content and actions; the workspace owns the navigation
/// and top bar. Detail routes can still use a normal back button.
class WorkspaceSection extends StatelessWidget {
  const WorkspaceSection({
    super.key,
    required this.title,
    required this.body,
    this.actions = const [],
  });

  final Widget title, body;
  final List<Widget> actions;

  @override
  Widget build(BuildContext context) {
    final info = context
        .dependOnInheritedWidgetOfExactType<WorkspaceSectionInfo>();
    if (info == null || ModalRoute.of(context)?.isFirst == false) {
      return Scaffold(
        appBar: AppBar(title: title, actions: actions),
        body: body,
      );
    }
    final compact = MediaQuery.sizeOf(context).width < 700;
    final colors = FrameColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: EdgeInsets.fromLTRB(20, compact ? 24 : 30, 20, 18),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.center,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      info.eyebrow,
                      style: TextStyle(
                        fontSize: 10,
                        letterSpacing: 1.5,
                        color: colors.muted,
                      ),
                    ),
                    const SizedBox(height: 8),
                    DefaultTextStyle(
                      style: TextStyle(
                        fontSize: compact ? 24 : 28,
                        height: 1.3,
                        fontWeight: FontWeight.w600,
                        color: colors.text,
                      ),
                      child: title,
                    ),
                    const SizedBox(height: 8),
                    Text(
                      info.description,
                      style: TextStyle(
                        fontSize: 12,
                        height: 1.6,
                        color: colors.muted,
                      ),
                    ),
                  ],
                ),
              ),
              if (actions.isNotEmpty) ...[
                const SizedBox(width: 12),
                ...actions,
              ],
            ],
          ),
        ),
        Expanded(child: body),
      ],
    );
  }
}
