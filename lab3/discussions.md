## CPSC 426 - Lab 3 Discussion

#### Bryan SebaRaj

#### 10/30/2024

#### 3A-2

##### 1. Construct and describe a scenario with a Raft cluster of 3 or 5 nodes where the leader election protocol fails to elect a leader.

Consider a Raft cluster with 3 nodes, $n_1$, $n_2$, and $n_3$. All nodes are initialized as
followers (default Raft behavior), and their election timeouts are manipulated by the byzantine
agent such that all nodes time out (and become candidates) simulataneously. As such, in the initial
absense of a leader, all nodes will timeout simultaneously and become a candidate, incrementing its
term and sending out RequestVote RPCs to the other nodes.

Consider the possiblity that all messages between (to and from) $n_1$ to $n_2$ and between $n_1$ and
$n_3$ are delayed indefinitely (which is an allowed state in an asynchronous, lossless network),
where only messages between $n_2$ and $n_3$ are delivered reasonably. Since $n_1$'s RequestVote do
not reach $n_2$ and $n_3$ before its candidate timeout expires, it receives zero votes. Similarly,
the RequestVote messages from $n_2$ and $n_3$ do not reach $n_1$ before their candidate timeout.
$n_2$ and $n_3$ receive each other's RequestVote RPCs, but since they are both already candidates,
they have voted for themselves and do not grant their votes to the other since they are in the same
term. As such, no node receives a majority of votes and the election fails, all nodes independently
(but simultaneously) timeout, increment their own terms, and start a new election, resulting in an
infinite loop of failed elections.

##### 2. In practice, why is this not a major concern? i.e., how does Raft get around this theoretical possibility?

In practice, this is not a major concern as an omniscient/omnipotent byzantine agent is not feasible when an
organization has control over the whole network. As such, the random election timeouts can be
truly(ish) random, reducing the likelihood that multiple nodes become candidates simultaneously.
In the real world, message delays are typically not manipulated and messages are often delivered
within predicated time frames. If the time frames fall outside of the eelction timeout, the raft nodes
can utilize a backoff strategy, increasing their timesouts to further reduce collision chances.
In addition, factors beyond the control of the raft algorithm, such as hardware clock differences or
thread/process scheduling by the operating system for each node introduces non-determinism that
benefits the election process.

#### ExtraCredit1

The scenario constructed in 3A-2.1 does not resolve if the Raft instances implement Pre-Vote and
CheckQuorum. Consider the same scenario with 3 nodes, initialized to followers. With the byzantine
agent controlling election timeouts, all nodes timeout simultaneously and enter the pre-vote phase,
sending their PreVoteRequests to each other. $n_1$ fails to receive PreVoteResponses from $n_2$ and
$n_3$ due to message delays, and $n_2$ and $n_3$ fail to receive PreVoteResponses from $n_1$. $n_2$
and $n_3$ can form a quorum each, and receive a pre-vote from the other (and themselves), ultimately
both simulatenously becoming candidates and starting a new election. Now identical to the scenario
in 3A-2.1, the election fails and nodes $n_2$ and $n_3$ enter an infinite loop of successful
PreVotes and failed elections. As a leader is never successfully elected, CheckQuorum will never be
utilized by the leader to ensure that it has a quorum of nodes to maintain its leadership.
