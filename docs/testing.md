# Testing strategy

This document maps every test category in the design spec section 10 to a
concrete, named test. It is the checklist the project-level definition of done
in `AGENTS.md` refers to.

Test names below are the intended Go identifiers; files are relative to the
repository root.

## Test style

Every test is one Given / When / Then:

- **Given:** build the fixture with helpers. Setup is not under test; a setup
  failure is `t.Fatal`.
- **When:** exactly one action under test. A read that only observes the
  outcome (for example `show` after `edit`) is part of Then, not a second When.
- **Then:** assert only on that action's outcome. Several assertions about the
  same outcome are fine.

A scenario with a second action is a second test. Prefer a fresh fixture per
test over a shared one; a test never depends on another test's state.

Name a test `Test<When>_<Given>_<Then>` in CamelCase parts, for example
`TestStart_OpenTicket_BecomesInProgress` or
`TestDepAdd_BackEdge_RefusedNamingCycleAndNotWritten`. Drop `<Given>` when the
fixture is trivial: `TestVersion_PrintsOneLine`.

Table tests run each case in `t.Run(name, ...)`, so one failing case does not
hide the rest and a case can be run alone with `-run`. Property, concurrency,
and crash tests keep one randomized or racing When per test.

## 1. Unit tests

### 1.1 Lock

| Spec item | File | Test name |
|---|---|---|
| Exclusive lock, bounded retry, timeout | `internal/lock/timeout_test.go` | `TestAcquire_HeldLock_TimesOutAndLeavesFilesUntouched` |
| Timeout error is typed | `internal/lock/timeout_test.go` | `TestAcquire_HeldLock_ReturnsErrTimeout` |
| Lock release and re-acquire | `internal/lock/lock_test.go` | `TestAcquire_FreeLock_IsHeld`, `TestRelease_HeldLock_IsNoLongerHeldAndKeepsFile`, `TestAcquire_AfterRelease_Succeeds` |
| Double release is visible | `internal/lock/lock_test.go` | `TestRelease_ReleasedLock_ReturnsErrNotHeld` |

### 1.2 Atomic write

| Spec item | File | Test name |
|---|---|---|
| Whole old-or-new file after replace | `internal/store/atomic_test.go` | `TestWriteDetailBytes_ConcurrentReader_NeverSeesPartialFile` |
| File mode preserved across replace | `internal/store/atomic_test.go` | `TestWriteDetailBytes_ExistingFile_PreservesModeAndReplacesContent` |
| Temp file is cleaned up | `internal/store/atomic_test.go` | `TestWriteDetailBytes_NewFile_LeavesNoTempFile` |
| Stale temp files swept under the lock | `internal/store/cleanup_test.go` | `TestSweepStaleTemp_TempAndRealFiles_RemovesOnlyTemp`, `TestSweepStaleTemp_MissingDirs_Succeeds`, `TestSweepStaleTemp_TempNamedDirectory_KeepsIt`, `TestBegin_StaleTempFile_SweepsBeforeReturning` |
| Read error is not an empty backlog | `internal/store/detail_test.go` | `TestReadDetail_MissingFile_ReturnsErrNotFound`, `TestReadDetail_DirectoryInPlaceOfFile_ReturnsReadError` |
| Corrupt detail is surfaced, not skipped | `internal/store/detail_test.go` | `TestListActive_CorruptDetail_ReturnsParseError` |
| Ticket is located in active or archive | `internal/store/detail_test.go` | `TestReadDetail_ArchivedFile_FindsArchivedCopy` |
| Detail write/read round-trip | `internal/store/detail_test.go` | `TestWriteDetail_TicketWithDeps_ReadsBackSameFieldsWithSortedDeps` |
| Root discovery walks up, creates nothing | `internal/store/detail_test.go` | `TestDiscover_NestedDir_FindsTicketsRoot`, `TestDiscover_GitRootWithoutTickets_CreatesNothing` |
| Missing `.tickets/`: read vs write | `internal/store/detail_test.go` | `TestRequireTickets_MissingDir_FailsAndCreatesNothing`, `TestBegin_MissingTicketsWithoutCreate_FailsWithErrNoTickets`, `TestBegin_MissingTicketsWithCreate_CreatesTickets` |
| Transaction reindex wiring | `internal/store/tx_test.go` | `TestCommit_WrittenDetail_RunsReindexOnceAfterWrite`, `TestRollback_AfterCommit_IsNoOp` |
| Store-level lock timeout changes nothing | `internal/store/tx_test.go` | `TestBegin_LockHeld_TimesOutAndCreatesNothing` |
| Begin requires `.tickets/` unless Create | `internal/store/tx_test.go` | `TestBegin_MissingTicketsWithoutCreate_FailsWithErrNoTickets`, `TestBegin_MissingTicketsWithCreate_CreatesTickets` |

### 1.3 ID uniqueness

| Spec item | File | Test name |
|---|---|---|
| Grammar rules | `internal/ids/grammar_test.go` | `TestValidate_GrammarTable_AcceptsOrRejectsPerRule` |
| Case folding before use | `internal/ids/grammar_test.go` | `TestNormalize_MixedCaseID_FoldsToLowercase`, `TestNormalize_NonCaseViolation_Rejects` |
| Reserved names rejected | `internal/ids/grammar_test.go` | `TestValidate_ReservedName_ReturnsErrReserved`, `TestNormalize_ReservedNameInAnyCase_ReturnsErrReserved` |
| Duplicate explicit ID rejected | `internal/ids/grammar_test.go` | `TestCheckAvailable_TakenIDInOtherCase_ReturnsErrDuplicate`, `TestCheckAvailable_FreeID_ReturnsNil`, `TestCheckAvailable_BadGrammar_ReturnsErrGrammar`, `TestCheckAvailable_NilPredicate_ReturnsError` |
| Existence helper canonicalizes | `internal/ids/grammar_test.go` | `TestExists_UppercaseID_QueriesLowercaseForm` |
| Generation never duplicates or reserves | `internal/ids/generate_test.go` | `TestGenerate_RandomDraws_AreValidUnreservedAndUnique` |
| Generation never emits reserved names | `internal/ids/generate_internal_test.go` | `TestGeneratedLengths_ReservedNames_AreUnreachable` |
| Generation retries then exhausts on collision | `internal/ids/generate_test.go` | `TestGenerate_EveryIDTaken_ReturnsErrGenerateExhausted` |
| Filesystem-backed existence predicate | `internal/ids/generate_test.go` | `TestGenerate_DirExistsWithFirstDrawTaken_SkipsExistingFile` |
| Partial-ID resolution and ambiguity | `internal/ids/resolve_test.go` | `TestResolve_OperandTable_ResolvesExactUniqueOrFails` |
| Exact match wins over substring | `internal/ids/resolve_test.go` | `TestResolve_ExactIDThatIsAlsoSubstring_ReturnsExactMatch` |
| Ambiguity lists sorted candidates | `internal/ids/resolve_test.go` | `TestResolve_AmbiguousOperand_ListsCandidatesSorted` |
| Resolution returns stored form | `internal/ids/resolve_test.go` | `TestResolve_UppercaseOperand_ReturnsStoredForm` |

### 1.4 Parser and serializer round-trip

| Spec item | File | Test name |
|---|---|---|
| Byte-stable round-trip on fixtures | `internal/frontmatter/roundtrip_test.go` | `TestSerializeOfParse_GoodFixture_EqualsCanonicalize`, `TestRoundTrip_SerializedFixture_IsByteIdenticalAndKeepsContent` |
| Second canonicalize pass is a no-op | `internal/frontmatter/roundtrip_test.go` | `TestCanonicalize_CanonicalFixture_IsFixedPoint` |
| Canonical field order and sorted lists | `internal/frontmatter/roundtrip_test.go` | `TestCanonicalize_GoodFixture_EmitsCanonicalFrontmatter` |
| Canonical timestamp form | `internal/model/ticket_test.go` | `TestFormatTimestamp_UTCTime_RendersCanonicalForm` |

## 2. Frontmatter subset tests (spec section 8.2)

| Spec item | File | Test name |
|---|---|---|
| Writer output valid under a reference YAML parser | `internal/frontmatter/property_yamlref_test.go` | `TestSerialize_RandomTicket_DecodesIdenticallyUnderYAMLv3` (build tag `yamlref`, `yaml.v3` test-only) |
| Production code does not import a YAML library | `internal/frontmatter/property_test.go` | `TestImports_ProductionGoFiles_ExcludeYAML` |
| Out-of-subset input fails loudly, with no document | `internal/frontmatter/reject_test.go` | `TestParse_NonIntegerPriority_FailsWithNoDocument`, `TestParse_OutOfSubsetFixture_ReturnsParseError` (table over `|`, `>`, `&x`, `*x`, `<<`, `{}`, nested maps, multi-line arrays, `null`, `~`, `yes`/`no`/`on`/`off`/`true`/`false`, duplicate keys, tabs, trailing junk) |
| Every reject fixture is covered by the reject table | `internal/frontmatter/reject_test.go` | `TestRejectFixtures_EveryFile_IsListedInRejectedForms` |
| Scalars are quoted only when unsafe and parse back unchanged | `internal/frontmatter/preserve_test.go` | `TestQuoteIfNeeded_Scalar_QuotesOnlyUnsafeTokens`, `TestParseScalar_QuotedScalar_RoundTrips` |
| Unknown/user keys preserved byte-for-byte | `internal/frontmatter/preserve_test.go` | `TestParse_UnknownKeys_CapturesThemVerbatimInOrder`, `TestSerialize_ParsedUnknownKeys_EmitsThemVerbatimInOrder` |
| Known keys always emitted, even when empty | `internal/frontmatter/preserve_test.go` | `TestSerialize_DefaultTicket_EmitsCoreKeysAndOmitsUnsetConditionalKeys` |
| Body-only file gets defaults, never a data-loss error | `internal/frontmatter/fence_test.go` | `TestParse_NoLeadingFence_DefaultsFieldsAndKeepsBodyVerbatim` |
| Body may contain `---` freely; byte-zero and CRLF fences | `internal/frontmatter/fence_test.go` | `TestParse_DashLinesInBody_KeepsWholeBody`, `TestParse_BlankLineBeforeFence_YieldsDefaults`, `TestParse_CRLFFences_ParsesFields` |
| An opened but unclosed fence is an error | `internal/frontmatter/fence_test.go` | `TestParse_UnclosedFence_ReturnsError` |

## 3. Index byte-stability (spec section 3.1)

| Spec item | File | Test name |
|---|---|---|
| `f(f(details)) == f(details)` | `internal/index/idempotence_test.go` | `TestRender_ReorderedInput_ProducesSameBytes`, `TestRender_SameDetailsTwice_ProducesSameBytes`, `TestRenderEntries_ParsedRender_ReproducesBytes`, `TestRender_ArchivedDetail_IsOmitted` |
| Two reindex runs produce no diff | `internal/index/golden_test.go` | `TestReindex_AlreadyReindexedFixture_RewritesSameGoldenBytes` |
| Status change edits one line, reorders nothing | `internal/index/idempotence_test.go` | `TestRender_InSectionStatusChange_ChangesOnlyThatLine`, `TestRender_CrossSectionStatusChange_KeepsOtherLinesIdentical` |
| Ordering `(priority, created, id)` | `internal/index/idempotence_test.go` | `TestRender_DifferentPriority_ListsLowerPriorityFirst`, `TestRender_SamePriority_ListsEarlierCreatedFirst`, `TestRender_SamePriorityAndCreated_ListsLowerIDFirst` |
| Golden fixture backlog matches | `internal/index/golden_test.go` | `TestBuild_GoldenFixture_MatchesGoldenIndex` |
| Reindex wired through the store transaction | `internal/index/golden_test.go` | `TestCommit_InjectedIndexReindex_WritesIndexWithNewTicket` |
| Parse round-trip recovers id/glyph/title/deps | `internal/index/parse_test.go` | `TestParse_RenderedDetails_RecoversIDGlyphTitleAndDeps`, `TestParse_GoldenIndex_ReturnsEveryEntry`, `TestParse_HandEditedIndex_RecoversTitlesAndSkipsLinesWithoutID`, `TestParseIndex_MissingFile_ReturnsNoEntriesAndNoError`, `TestParse_EmptyBody_ReturnsNoEntries` |
| Parse tolerates a stale Archived section | `internal/index/parse_test.go` | `TestParse_StaleArchivedSection_KeepsEntriesWithoutArchivedMarker` |
| Navigation-only render preserves file order | `internal/index/parse_test.go` | `TestRenderEntries_UnsortedEntries_KeepsFileOrder` |

## 4. Multi-process / concurrency tests (spec section 10)

These exercise the store transaction directly. The `add` and `start` CLI
verbs call the same store-level primitives tested here.

| Spec item | File | Test name |
|---|---|---|
| N parallel `add` present once, unique IDs, no lost lines | `internal/store/parallel_add_test.go` | `TestParallelAdd_DistinctTickets_EachPresentOnceInStoreAndIndex` |
| Sequential adds are each listed once, sorted | `internal/store/parallel_add_test.go` | `TestListActive_SequentialAdds_ReturnsEachOnceSorted` |
| Two parallel `start` -> exactly one success | `internal/store/parallel_start_test.go` | `TestParallelStart_OpenTicket_ExactlyOneWinsAndLoserSeesInProgress` |
| Kill mid-transaction -> whole file, no temp left, next op works | `internal/store/crash_test.go` | `TestKill_MidTransaction_LeavesWholeFileAndNextTxSucceeds` |
| Lock timeout -> clean error, file untouched | `internal/lock/timeout_test.go` | `TestAcquire_HeldLock_TimesOutAndLeavesFilesUntouched` |

## 5. Property tests

| Spec item | File | Test name |
|---|---|---|
| Parser/serializer idempotence for arbitrary fields | `internal/frontmatter/property_test.go` | `TestSerializeOfParse_RandomTicket_IsFixedPointAndKeepsBody`, `TestCanonicalize_RandomTicket_IsFixedPoint` |
| DAG add sequence never closes a cycle | `internal/graph/dag_property_test.go` | `TestWouldCycle_RandomDepAdds_RefusesOnlyCyclesAndKeepsGraphAcyclic` |

## 5a. Dependency graph (spec sections 5.2 and 10)

| Spec item | File | Test name |
|---|---|---|
| Self-loop refused with the cycle path | `internal/graph/cycle_test.go` | `TestWouldCycle_SelfEdge_ReportsSelfLoop` |
| Three-node back edge refused | `internal/graph/cycle_test.go` | `TestWouldCycle_BackEdgeOnChain_ReportsFullCycle`, `TestWouldCycle_EdgeToIndependentNode_IsSafe` |
| Long path / diamond reachability | `internal/graph/cycle_test.go` | `TestWouldCycle_BackEdgeAcrossDiamond_IsDetected`, `TestWouldCycle_ExistingDiamondEdge_IsSafe` |
| Injected cycle found by the backstop | `internal/graph/cycle_test.go` | `TestFindCycle_ThreeNodeCycle_ReportsClosedPath`, `TestFindCycle_SelfLoop_ReportsAToA` |
| Acyclic graph reports no cycle | `internal/graph/cycle_test.go` | `TestFindCycle_Diamond_ReportsNone`, `TestFindCycle_DAGWithForwardAndCrossEdges_ReportsNone` |
| Tree terminates on an injected cycle | `internal/graph/tree_test.go` | `TestTree_InjectedCycle_TerminatesAndMarksCycle` |
| Tree shared dep printed once, "(seen)" | `internal/graph/tree_test.go` | `TestTree_Diamond_PrintsSharedDepOnceThenSeen` |
| Tree --full re-expands every node and still stops at a cycle | `internal/graph/tree_test.go` | `TestTreeFull_Diamond_ReExpandsSharedDep`, `TestTreeFull_InjectedCycle_StopsAtCycle` |
| Tree marks an unresolved dep | `internal/graph/tree_test.go` | `TestTree_UnknownDep_MarksItUnknown` |
| Unresolved warnings sorted by (ticket, dep) | `internal/graph/views_test.go` | `TestWarnings_UnresolvedDeps_SortedByTicketThenDep` |
| Dep adjacency sorted and unique | `internal/graph/views_test.go` | `TestDeps_UnsortedDuplicateDeps_ReturnsSortedUnique` |
| Dep order: dependency precedes dependent | `internal/graph/order_test.go` | `TestOrder_DependentPreferredFirst_EmitsDepFirst` |
| Dep order: independent tickets keep preference | `internal/graph/order_test.go` | `TestOrder_IndependentTickets_KeepsPreferenceOrder` |
| Dep order: chain drains in dependency order | `internal/graph/order_test.go` | `TestOrder_Chain_EmitsInDependencyOrder` |
| Dep order: diamond roots before sink | `internal/graph/order_test.go` | `TestOrder_Diamond_EmitsEveryDepBeforeItsDependent` |
| Dep order: cycle terminates, every ticket once, deterministic | `internal/graph/order_test.go` | `TestOrder_InjectedCycle_YieldsEveryTicketExactlyOnce`, `TestOrder_InjectedCycle_IsDeterministic` |
| Dep order: unresolved dep ignored | `internal/graph/order_test.go` | `TestOrder_UnresolvedDep_StillEmitsTicket` |
| CLI: index-only views preserve committed order | `internal/cli/dep_test.go` | `TestViews_IndexOutOfPriorityOrder_KeepIndexOrder` |
| CLI: text verbosity ladder (bare id / -v / -vv) on every view | `internal/cli/dep_test.go` | `TestViews_VerbosityFlags_PrintLadderLevel`, `TestStart_AvailableDefaultOutput_ResolvesAsOperand` |
| CLI: priority/limit flags removed from views | `internal/cli/dep_test.go` | `TestViews_RemovedFlags_FailAsUsageErrors` |
| CLI: dep add/rm and sorted mirror | `internal/cli/dep_test.go` | `TestDepAdd_DepAddedOutOfOrder_SortsDetailAndIndexMirror`, `TestDepRm_ExistingDep_RemovesOnlyThatDep`, `TestDepRm_SameDepTwice_SecondFails`, `TestDepRm_AbsentDep_FailsAndChangesNothing` |
| CLI: self-loop refused, nothing written | `internal/cli/dep_test.go` | `TestDepAdd_SelfLoop_RefusedNamingCycleAndNotWritten` |
| CLI: dep add refuses an unknown ticket on either side | `internal/cli/dep_test.go` | `TestDepAdd_UnknownDependency_FailsAndChangesNothing`, `TestDepAdd_UnknownSource_FailsAndChangesNothing` |
| CLI: 3-node back edge refused, nothing written | `internal/cli/dep_test.go` | `TestDepAdd_ThreeNodeBackEdge_RefusedNamingFullCycleAndNotWritten` |
| CLI: dep cycle backstop | `internal/cli/dep_test.go` | `TestDepCycle_CleanGraph_ReportsNoCycles`, `TestDepCycle_InjectedCycle_ReportsCycle` |
| CLI: dep tree terminates on injected cycle | `internal/cli/dep_test.go` | `TestDepTree_InjectedCycle_TerminatesAndMarksCycle` |
| CLI: dep order topo beats priority | `internal/cli/dep_order_test.go` | `TestDepOrder_UrgentTicketBlockedByLowPriority_ListsBlockerFirst` |
| CLI: dep order (priority, created, id) tiebreak | `internal/cli/dep_order_test.go` | `TestDepOrder_IndependentTickets_ListsByPriority` |
| CLI: dep order omits closed tickets | `internal/cli/dep_order_test.go` | `TestDepOrder_ClosedBlocker_ListsOnlyDependent`, `TestDepOrderJSON_ClosedBlocker_ListsOnlyDependent` |
| CLI: dep order omits archived, never warns on it | `internal/cli/dep_order_test.go` | `TestDepOrder_ArchivedBlocker_ListsOnlyDependentWithoutWarning` |
| CLI: dep order warns on unresolved dep, never stalls | `internal/cli/dep_order_test.go` | `TestDepOrder_UnresolvedDep_WarnsAndStillListsTicket` |
| CLI: dep order --json carries position | `internal/cli/dep_order_test.go` | `TestDepOrderJSON_BlockedAndBlocker_ListsOneBasedPositionsWithDeps` |
| CLI: dep order is a pure read | `internal/cli/dep_order_test.go` | `TestDepOrder_TicketsWithDep_LeavesIndexUnchanged` |
| CLI: dep order on missing .tickets errors with guidance | `internal/cli/dep_order_test.go` | `TestDepOrder_MissingTickets_FailsWithGuidanceAndCreatesNothing` |
| CLI: unresolved dep warning | `internal/cli/dep_test.go` | `TestAvailable_UnresolvedDep_WarnsAndExcludesTicket`, `TestDepTree_UnresolvedDep_MarksItMissing` |
| CLI: views across status combinations | `internal/cli/dep_test.go` | `TestViews_DepStatusStages_ListExpectedTickets` |
| CLI: reads on missing backlog | `internal/cli/dep_test.go` | `TestViewAndDepCommands_MissingTickets_FailWithGuidanceAndCreateNothing` |
| CLI: end-to-end dep lifecycle | `test/cli_test.go` | `TestDepAdd_TwoTickets_MirrorsDepsIntoIndex`, `TestDepCycle_AcyclicGraph_ReportsNoCycles`, `TestDepAdd_BackEdge_RefusedNamingCycleAndNotWritten`, `TestBlocked_OpenDep_ListsDependent`, `TestAvailable_OpenDep_ExcludesDependent`, `TestAvailable_ClosedDep_ListsDependent`, `TestDepTree_Dependent_PrintsDependency`, `TestDepOrder_ClosedDep_OmitsClosedKeepsDependent`, `TestDepAdd_SelfLoop_RefusedAndNotWritten`, `TestDepRm_Edge_ClearsDeps`, `TestClosed_ClosedTicket_IsListed`, `TestClosed_LimitFlag_FailsAsUnknownFlag` |

## 5b. Archive (spec section 5.4)

| Spec item | File | Test name |
|---|---|---|
| Location-field agreement predicate | `internal/store/archive_test.go` | `TestLocationMatchesField_LocationAndField_TrueOnlyWhenTheyAgree`, `TestArchiveAgreement_AllAgree_ReturnsNil`, `TestArchiveAgreement_ArchivePathWithoutField_ReportsIt` |
| A write follows the archived field into/out of archive/ | `internal/store/archive_test.go` | `TestWriteDetailMoved_ArchivedFieldSet_MovesFileToArchive`, `TestWriteDetailMoved_ArchivedFieldCleared_MovesFileBack` |
| A move never overwrites a duplicated ID's other copy | `internal/store/archive_test.go` | `TestWriteDetailMoved_DuplicateInArchive_RefusesAndKeepsBothCopies`, `TestWriteDetailMoved_DuplicateInActive_RefusesAndKeepsBothCopies` |
| Hot-path name-only archive/candidate listing | `internal/store/archive_test.go` | `TestListArchiveIDs_UnparseableArchive_ReturnsSortedStems`, `TestCandidateIDs_UnparseableArchive_ReturnsActiveAndArchivedSorted`, `TestListAll_UnparseableArchive_Fails` |
| `--closed-older-than` selection correctness | `internal/store/archive_test.go` | `TestArchiveCandidates_MixedTickets_SelectsOnlyOldClosed`, `TestArchiveCandidates_MalformedClosedAt_NeverQualifies` |
| Archiving an open ticket is an error, changes nothing | `internal/cli/archive_test.go` | `TestArchive_OpenTicket_FailsAndChangesNothing` |
| Agreement holds after archive/unarchive/reopen | `internal/cli/archive_test.go` | `TestArchive_ClosedTicket_MovesFileAndSetsField`, `TestArchive_ClosedTicket_DropsItFromIndex`, `TestUnarchive_ArchivedTicket_ClearsFieldKeepsClosedAtAndMovesBack`, `TestReopen_ArchivedTicket_ClearsArchivedAndClosedAtAndMovesBack` |
| Archiving removes only the archived line; every other index line stays in place | `internal/cli/archive_test.go` | `TestArchive_ClosedTicket_RemovesOnlyItsIndexLine` |
| `--dry-run` touches nothing | `internal/cli/archive_test.go` | `TestArchiveDryRun_ByID_ListsTicketAndChangesNothing`, `TestArchiveDryRun_ByAgeWithOldTicket_ListsTicketAndChangesNothing` |
| Age selection uses the injected clock | `internal/cli/archive_test.go` | `TestArchiveClosedOlderThan_MixedAges_ArchivesOnlyOlderClosed`, `TestArchiveClosedOlderThan_EachUnit_SplitsAtThreshold`, `TestArchiveClosedOlderThan_AlreadyArchivedTicket_SkipsIt` |
| Unarchive on a non-archived ticket changes nothing | `internal/cli/archive_test.go` | `TestUnarchive_ActiveTicket_FailsAndChangesNothing` |
| Archive/unarchive of a duplicated ID fails, changes nothing | `internal/cli/archive_test.go` | `TestArchive_DuplicatedID_FailsAndChangesNothing`, `TestUnarchive_DuplicatedID_FailsAndChangesNothing` |
| Hot-path views ignore archive/; --archived, --all, and doctor surface it | `internal/cli/archive_test.go` | `TestViews_CorruptArchivedDetail_StillSucceed`, `TestList_CorruptArchivedDetail_StillListsActiveTicket`, `TestListArchived_CorruptArchivedDetail_Fails`, `TestDoctor_CorruptArchivedDetail_Fails` |
| Partial-ID archive resolution | `internal/cli/archive_test.go` | `TestArchive_PartialID_ArchivesMatchingTicket` |
| CLI integration: end-to-end archive | `test/cli_test.go` | `TestArchive_ClosedTicket_MovesToArchive`, `TestArchive_OpenTicket_FailsAndStaysActive`, `TestUnarchive_ArchivedTicket_KeepsClosedAndMovesBack`, `TestReopen_ArchivedTicket_BecomesOpenAndMovesBack` |

## 5c. Attachments (spec section 3.5)

| Spec item | File | Test name |
|---|---|---|
| `__` delimiter unambiguous because underscore is forbidden in IDs | `internal/attach/attach_test.go` | `TestIDsValidate_Underscore_FailsWithErrGrammar`, `TestName_SuffixWithUnderscore_JoinsWithDoubleUnderscore`, `TestSplit_SuffixWithUnderscore_KeepsItInSuffix`, `TestSplit_MalformedName_ReportsNotOK` |
| Suffix may not escape the attachments dir (traversal) | `internal/attach/attach_test.go` | `TestValidateSuffix_PlainFilename_Accepts`, `TestValidateSuffix_PathOrEmpty_RejectsWithErrInvalidSuffix` |
| Source path contributes only its base name | `internal/attach/attach_test.go` | `TestSourceSuffix_NestedPath_ReturnsBaseName` |
| Copy/list/remove round-trip, lazy directory | `internal/attach/attach_test.go` | `TestCopy_FirstFile_CreatesDirAndStoresUnderIDSuffix`, `TestFilesFor_StoredFile_ListsItsSuffix`, `TestEntries_StoredLinkedFile_IsPresent`, `TestRemove_StoredFile_DeletesItAndReportsRemoved`, `TestRemove_MissingFile_IsNoOpWithoutError` |
| Attach never overwrites a different ticket's file | `internal/attach/attach_test.go` | `TestCopy_SameSuffixForAnotherTicket_KeepsBothFiles`, `TestFilesFor_PrefixSharingIDs_ListsOnlyExactIDFiles` |
| Doctor support: missing frontmatter entry / orphan file | `internal/attach/attach_test.go` | `TestCheckLinks_MissingAndOrphan_ReportsBoth`, `TestCheckLinks_LinksMatchFiles_ReportsNothing`, `TestCheckLinks_NoDirAndNoLinks_ReportsNothing`, `TestCheckLinks_NoDirWithLink_ReportsMissing` |
| Attachment entries are unique, sorted, and deterministic | `internal/attach/attach_test.go` | `TestUniqueSorted_UnsortedDuplicates_ReturnsSortedUnique` |
| CLI: attach/detach/attachments/show --attachments round-trip | `internal/cli/attach_test.go` | `TestAttach_TwoFiles_ReportsCopiesStoresBytesAndLinksSorted`, `TestAttachments_TwoAttached_ListsSorted`, `TestShowAttachments_TwoAttached_AppendsBlock`, `TestDetach_AttachedFile_RemovesFileAndLink`, `TestDetach_AlreadyDetached_FailsWithNoAttachment` |
| CLI: dir optional and created lazily | `internal/cli/attach_test.go` | `TestAdd_Ticket_CreatesNoAttachmentsDir`, `TestAttachments_NoAttachments_ReportsNone`, `TestShowAttachments_NoAttachments_ReportsNone`, `TestAttach_FirstFile_CreatesAttachmentsDir` |
| CLI: same ticket+suffix is an explicit replace, not a cross-ticket overwrite | `internal/cli/attach_test.go` | `TestAttach_SameTicketAndSuffix_ReplacesBytesAndReportsIt`, `TestAttach_SameSuffixOnOtherTicket_LeavesFirstTicketsFile` |
| CLI: detach repairs either side (frontmatter-only / orphan-only) | `internal/cli/attach_test.go` | `TestDetach_LinkWithoutFile_ClearsLink`, `TestDetach_OrphanFile_RemovesFile` |
| CLI: missing link / orphan surfaced | `internal/cli/attach_test.go` | `TestAttachments_MissingAndOrphan_FlagsBoth`, `TestShowAttachments_MissingAndOrphan_FlagsBoth` |
| CLI: traversal suffix rejected, nothing written | `internal/cli/attach_test.go` | `TestAttach_TraversalSuffix_FailsAndChangesNothing`, `TestDetach_TraversalSuffix_FailsAndChangesNothing` |
| CLI: underscore ID is a grammar error | `internal/cli/attach_test.go` | `TestAttach_UnderscoreID_FailsAndCreatesNothing` |
| Hot paths and mutations never need attachments/ | `internal/cli/attach_test.go` | `TestViews_AttachmentsDirRemoved_AnswerFromIndex`, `TestStart_AttachmentsDirRemoved_DoesNotRecreateIt` |
| CLI: partial/archived ID resolution | `internal/cli/attach_test.go` | `TestAttach_PartialArchivedID_StoresFileAndLink`, `TestAttachments_PartialArchivedID_ListsFile` |
| CLI integration: end-to-end attachments | `test/cli_test.go` | `TestAttach_File_CopiesSidecarAndLinksFrontmatter`, `TestAttachments_AttachedTicket_ListsFile`, `TestShowAttachments_AttachedTicket_PrintsBlock`, `TestDetach_AttachedFile_RemovesSidecarAndLink`, `TestAttachments_NoAttachments_ReportsNone`, `TestDetach_UnknownAttachment_FailsWithNoAttachment`, `TestAttach_DotDotSuffix_FailsAsInvalidSuffix` |

## 6. CLI tests (spec section 7)

| Spec item | File | Test name |
|---|---|---|
| CLI integration: usage/dispatch | `test/cli_test.go` | `TestNoArgs_PrintsUsage`, `TestHelp_PrintsUsage`, `TestUnknownCommand_FailsWithUsageOnStderr` |
| CLI in-process: no-args usage and unknown command | `internal/cli/app_test.go` | `TestAppRun_NoArgs_PrintsUsage`, `TestAppRun_UnknownCommand_ExitsUsageWithMessage` |
| CLI: timestamps come from the injected clock; a transition bumps only updated | `internal/cli/app_test.go` | `TestAdd_InjectedClock_StampsCreatedAndUpdatedWithIt`, `TestStart_AdvancedClock_BumpsUpdatedAndKeepsCreated` |
| CLI integration: add/list/show/edit | `test/cli_test.go` | `TestAdd_FreshDir_CreatesTicketsAndIndex`, `TestList_MixedPriorities_SortsByPriority`, `TestShow_GeneratedID_PrintsTicket`, `TestShow_UppercasePartialID_MatchesFullID`, `TestEdit_PriorityAndNote_UpdatesDetail`, `TestEdit_Priority_RegeneratesIndexOrder` |
| CLI integration: guarded transitions | `test/cli_test.go` | `TestStart_OpenTicket_BecomesInProgress`, `TestStart_InProgressTicket_FailsAndKeepsStatus`, `TestReopen_InProgressTicket_BecomesOpen`, `TestDone_OpenTicket_FailsAndKeepsStatus`, `TestDone_InProgressTicket_ClosesAndSetsClosedAt`, `TestDone_ClosedTicket_FailsAndKeepsStatus`, `TestReopen_ClosedTicket_BecomesOpenAndClearsClosedAt`, `TestReopen_OpenTicket_FailsAndKeepsStatus`, `TestStatus_ExistingTicket_FailsAsUnknownCommand` |
| CLI integration: list filters (index-only, no -p) | `test/cli_test.go` | `TestList_Default_ShowsOnlyActive`, `TestList_StatusClosed_ShowsOnlyClosed`, `TestList_All_ShowsEveryTicket`, `TestList_PriorityFlag_FailsAsUnknownFlag`, `TestList_BogusStatus_Fails` |
| CLI: list and views read backlog.md only; missing detail still flagged by doctor | `test/cli_test.go` | `TestViews_DetailDeleted_StillSucceed`, `TestListAll_DetailDeleted_StillListsTicket`, `TestDoctor_DetailDeleted_Fails` |
| CLI integration: explicit IDs | `test/cli_test.go` | `TestAdd_DuplicateID_FailsAndWritesNothing`, `TestAdd_UnderscoreID_FailsAndWritesNothing`, `TestAdd_ReservedID_FailsAndWritesNothing` |
| CLI integration: empty backlog | `test/cli_test.go` | `TestList_EmptyBacklog_PrintsEmptySentinel` |
| CLI integration: basic --json | `test/cli_test.go` | `TestList_NoJSONFlag_PrintsPlainText`, `TestListJSON_OneTicket_PrintsVersionedTicket`, `TestListJSON_OneTicket_OmitsDetailFields`, `TestShowJSON_Ticket_PrintsFullDetailAndBody` |
| Concurrency through the CLI | `test/cli_test.go` | `TestParallelStart_OpenTicket_ExactlyOneSucceeds` |
| Missing `.tickets/` read vs write | `test/cli_test.go` | `TestNonAddCommands_MissingTickets_FailWithGuidanceAndCreateNothing`, `TestAdd_FreshDir_CreatesTicketsAndIndex` |
| Mutating command sweeps a stale temp file | `test/cli_test.go` | `TestAdd_StaleTempFile_SweepsIt` |
| Body section editor | `internal/cli/render_test.go` | `TestSetSection_Replace_ReplacesOnlyTargetSection`, `TestSetSection_Append_AddsBlockAfterExistingText`, `TestSetSection_EmptyText_RemovesSectionAndHeading`, `TestSetBodyTitle_ExistingHeading_ReplacesItAndKeepsContent`, `TestSetBodyTitle_NoHeading_PrependsItAndKeepsContent` |
| CLI flag parser | `internal/cli/render_test.go` | `TestFlagSet_InterleavedFlagsAndPositionals_ParsesValuesAndTracksSet`, `TestFlagSet_BadInput_FailsToParse`, `TestFlagSet_DoubleDash_TreatsRestAsPositionals` |

## 7. JSON output (spec section 7)

| Spec item | File | Test name |
|---|---|---|
| Schema snapshot per read command (golden files under `internal/cli/testdata/json/`) | `internal/cli/schema_test.go` | `TestJSON_EveryReadCommand_MatchesGolden` |
| Version field present and stable; lean view tickets carry id/status/title/deps/archived and no detail fields | `internal/cli/schema_test.go` | `TestJSON_EveryCommand_CarriesVersionAndCommand`, `TestListJSON_OneTicket_EmitsArrayOfLeanItems` |
| Show content hash present and stable; changes when a detail changes | `internal/cli/schema_test.go` | `TestShowJSON_UnchangedDetail_RepeatsHash`, `TestShowJSON_EditedDetail_ChangesHash` |
| Show is the full detail shape (priority/created/hash) | `internal/cli/schema_test.go` | `TestShowJSON_OneTicket_CarriesFullDetailFields` |
| Output identical with stdin piped vs a regular file (no tty branching) | `test/cli_test.go` | `TestReadCommands_PipedOrFileStdin_PrintSameBytesAndCode` |

The navigation views (`list`, `started`, `blocked`, `available`, `closed`) are
index-only and emit the lean shape `{id, status, title, deps, archived}`. The
default, `--status`, and the dependency views read `backlog.md` alone, preserve
the committed order, and have no priority filter and no closed `--limit`;
`list --archived`/`--all` read `.tickets/archive/` because archived tickets are
not in the index (§3.1). `show --json` is the one full-detail response
and still carries priority, timestamps, attachments, and the content hash.

`--json` byte-identity is asserted in the subprocess suite because tty state
belongs to the process, not the handler. The subprocess always pipes stdout
(`test/cli_test.go` buffers), and `TestReadCommands_PipedOrFileStdin_PrintSameBytesAndCode` re-runs every read
command with stdin attached to a regular file and compares the stdout bytes and
exit codes. A read command must never consult `isatty`/`Stat` to choose a
format, so the two runs are equal.

## 8. Doctor (spec sections 3.1, 5.2, 5.4, 6.4)

| Spec item | File | Test name |
|---|---|---|
| A repo built through the write path is clean | `internal/doctor/doctor_test.go` | `TestRun_CleanRepo_ReportsNoIssues` |
| Filename/ID integrity | `internal/doctor/doctor_test.go` | `TestRun_BadFilename_FlagsFilename` |
| Uniqueness across active + archive | `internal/doctor/doctor_test.go` | `TestRun_SameIDInActiveAndArchive_FlagsDuplicate` |
| Index <-> detail consistency | `internal/doctor/doctor_test.go` | `TestRun_TamperedIndex_FlagsIndexDriftAsError` |
| Dep integrity (warning, never silent) | `internal/doctor/doctor_test.go` | `TestRun_UnknownDep_FlagsDepAsWarningOnly` |
| Cycle backstop | `internal/doctor/doctor_test.go` | `TestRun_CycleWrittenToDisk_FlagsCycle` |
| Archive location <-> field agreement | `internal/doctor/doctor_test.go` | `TestRun_UnmarkedFileInArchive_FlagsArchiveMismatch` |
| Attachment orphan file | `internal/doctor/doctor_test.go` | `TestRun_OrphanAttachmentFile_FlagsAttachment` |
| Attachment missing file | `internal/doctor/doctor_test.go` | `TestRun_DanglingAttachmentLink_FlagsAttachment` |
| `doctor fix` repairs the index and never rewrites unrelated details | `internal/doctor/doctor_test.go` | `TestFix_TamperedIndex_RebuildsIndexAndLeavesDetailsByteIdentical` |
| `doctor fix` is idempotent | `internal/doctor/doctor_test.go` | `TestFix_CleanRepo_ReportsNoWork`, `TestFix_AlreadyFixedRepo_ReportsNoWork` |
| `doctor fix` drops an unresolved dep | `internal/doctor/doctor_test.go` | `TestFix_UnknownDep_DropsItFromDeps` |
| `doctor fix` reconciles attachment link/orphan both ways | `internal/doctor/doctor_test.go` | `TestFix_DanglingLink_DropsIt`, `TestFix_OrphanFile_AdoptsIt` |
| `doctor fix` reconciles archive location↔field | `internal/doctor/doctor_test.go` | `TestFix_UnmarkedFileInArchive_MovesItBackToActive` |
| `doctor fix` leaves identity corruption for a human | `internal/doctor/doctor_test.go` | `TestFix_BadFilename_LeavesFileAndFinding`, `TestFix_DuplicateID_LeavesBothCopiesAndFinding` |
| CLI: doctor clean, drift, and fix | `internal/cli/maintenance_test.go` | `TestDoctor_CleanRepo_ExitsOKWithCleanReport`, `TestDoctor_TamperedIndex_ExitsErrorWithFinding`, `TestDoctorFix_TamperedIndex_ReportsRepairAndLeavesDetail` |
| CLI: `bl reindex` regenerates the index | `internal/cli/maintenance_test.go` | `TestReindex_JunkIndex_RegeneratesItFromDetails` |
| CLI: version is a single stable line | `internal/cli/maintenance_test.go` | `TestVersion_InProcess_PrintsOneBlPrefixedLine` |
| CLI end-to-end: version, doctor clean/drift/fix, reindex | `test/cli_test.go` | `TestVersion_PrintsOneLine`, `TestDoctor_CleanRepo_ReportsClean`, `TestDoctor_TamperedIndex_Fails`, `TestDoctorFix_TamperedIndex_RegeneratesIndexAndLeavesClean`, `TestReindex_TamperedIndex_LeavesClean` |

## Running

`make check` runs `go vet ./...`, `golangci-lint run`, and `go test ./...` in
that order. The race detector is not part of the gate: `bl` has no in-process
concurrency, and the concurrency it guards against is between processes, which
the concurrency tests above cover.

End-to-end CLI coverage is the plain Go subprocess suite in `test/cli_test.go`:
it builds `bl` once and drives it in temp directories. There is no BDD layer.

The reference-YAML property test is behind the `yamlref` build tag and is **not**
part of `make check`. Run it explicitly:

```
go test -tags yamlref ./internal/frontmatter/...
```

`gopkg.in/yaml.v3` is a test-only dependency: it is imported only from files
with the `yamlref` build tag, and `TestImports_ProductionGoFiles_ExcludeYAML` guards
this by scanning every non-test `.go` file in the module.
