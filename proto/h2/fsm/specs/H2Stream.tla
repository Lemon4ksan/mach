--------------------------- MODULE H2Stream ---------------------------
EXTENDS TLC, Sequences

\* =====================================================================
\* CONSTANTS & VARIABLES
\* =====================================================================

\* The set of possible states for an HTTP/2 stream (RFC 7540).
\* We omit Push Promise (RESERVED_LOCAL/REMOTE) for simplicity.
STATES == {
    "IDLE", 
    "OPEN", 
    "HALF_CLOSED_LOCAL", 
    "HALF_CLOSED_REMOTE", 
    "CLOSED"
}

VARIABLES state
vars == <<state>>

\* =====================================================================
\* INVARIANTS (SAFETY)
\* =====================================================================

\* Type invariant: the state must always be one of the known STATES.
TypeOK == state \in STATES

\* =====================================================================
\* INITIALIZATION & ACTIONS
\* =====================================================================

\* A stream always starts in the IDLE state.
Init == state = "IDLE"

\* ACTION: Local sends HEADERS (starts the stream)
SendHeaders == 
    /\ state = "IDLE"
    /\ state' = "OPEN"

\* ACTION: Remote sends HEADERS (starts the stream)
RcvHeaders == 
    /\ state = "IDLE"
    /\ state' = "OPEN"

\* ACTION: Local sends END_STREAM flag (finishes its half of the connection)
SendEndStream ==
    \/ /\ state = "OPEN"
       /\ state' = "HALF_CLOSED_LOCAL"
    \/ /\ state = "HALF_CLOSED_REMOTE"
       /\ state' = "CLOSED"

\* ACTION: Remote sends END_STREAM flag (finishes its half of the connection)
RcvEndStream ==
    \/ /\ state = "OPEN"
       /\ state' = "HALF_CLOSED_REMOTE"
    \/ /\ state = "HALF_CLOSED_LOCAL"
       /\ state' = "CLOSED"

\* ACTION: Either endpoint sends or receives a RST_STREAM frame (abort)
AbortStream ==
    /\ state \notin {"CLOSED", "IDLE"}
    /\ state' = "CLOSED"

\* =====================================================================
\* SYSTEM SPECIFICATION
\* =====================================================================

\* NEXT STATE: The system moves to the next state if ANY of these actions occur.
\* If it reaches CLOSED, it stays CLOSED forever (allowed stuttering to prevent deadlock errors).
Next == 
    \/ SendHeaders
    \/ RcvHeaders
    \/ SendEndStream
    \/ RcvEndStream
    \/ AbortStream
    \/ (state = "CLOSED" /\ UNCHANGED vars)

\* FAIRNESS: We assume the application and network will eventually make progress.
\* Weak Fairness (WF) means if an action is continuously possible, it will eventually happen.
Fairness == WF_vars(Next)

\* The full specification binds the initial state, the next state relations, and fairness.
Spec == Init /\ [][Next]_vars /\ Fairness

\* =====================================================================
\* PROPERTIES (LIVENESS)
\* =====================================================================

\* LIVENESS: If we continuously process frames, the stream will ALWAYS eventually reach CLOSED.
EventuallyClosed == <> (state = "CLOSED")

=============================================================================
