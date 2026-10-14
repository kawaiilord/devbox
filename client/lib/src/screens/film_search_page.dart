import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import 'catalog_browser.dart';
import 'metadata_search_page.dart';
import 'workspace_section.dart';

class FilmSearchPage extends StatefulWidget {
  const FilmSearchPage({
    super.key,
    required this.api,
    required this.session,
    required this.onOpenRoom,
  });
  final ApiClient api;
  final Session session;
  final ValueChanged<Room> onOpenRoom;
  @override
  State<FilmSearchPage> createState() => _FilmSearchPageState();
}

class _FilmSearchPageState extends State<FilmSearchPage> {
  bool _metadata = false;
  @override
  Widget build(BuildContext context) => WorkspaceSection(
    title: const Text('影视搜索'),
    body: Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
          child: SegmentedButton<bool>(
            segments: const [
              ButtonSegment(
                value: false,
                label: Text('片源搜索'),
                icon: Icon(Icons.play_circle_outline_rounded),
              ),
              ButtonSegment(
                value: true,
                label: Text('资料与影评'),
                icon: Icon(Icons.rate_review_outlined),
              ),
            ],
            selected: {_metadata},
            onSelectionChanged: (values) =>
                setState(() => _metadata = values.single),
          ),
        ),
        Expanded(
          child: _metadata
              ? MetadataSearchPage(
                  api: widget.api,
                  session: widget.session,
                  embedded: true,
                )
              : CatalogBrowser(
                  api: widget.api,
                  session: widget.session,
                  onOpenRoom: widget.onOpenRoom,
                ),
        ),
      ],
    ),
  );
}
