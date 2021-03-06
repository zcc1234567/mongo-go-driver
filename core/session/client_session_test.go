package session

import (
	"testing"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/core/uuid"
	"github.com/mongodb/mongo-go-driver/internal/testutil/helpers"
	"github.com/stretchr/testify/require"
)

func compareOperationTimes(t *testing.T, expected *bson.Timestamp, actual *bson.Timestamp) {
	if expected.T != actual.T {
		t.Fatalf("T value mismatch; expected %d got %d", expected.T, actual.T)
	}

	if expected.I != actual.I {
		t.Fatalf("I value mismatch; expected %d got %d", expected.I, actual.I)
	}
}

func TestClientSession(t *testing.T) {
	var clusterTime1 = bson.NewDocument(bson.EC.SubDocument("$clusterTime",
		bson.NewDocument(bson.EC.Timestamp("clusterTime", 10, 5))))
	var clusterTime2 = bson.NewDocument(bson.EC.SubDocument("$clusterTime",
		bson.NewDocument(bson.EC.Timestamp("clusterTime", 5, 5))))
	var clusterTime3 = bson.NewDocument(bson.EC.SubDocument("$clusterTime",
		bson.NewDocument(bson.EC.Timestamp("clusterTime", 5, 0))))

	t.Run("TestMaxClusterTime", func(t *testing.T) {
		maxTime := MaxClusterTime(clusterTime1, clusterTime2)
		if maxTime != clusterTime1 {
			t.Errorf("Wrong max time")
		}

		maxTime = MaxClusterTime(clusterTime3, clusterTime2)
		if maxTime != clusterTime2 {
			t.Errorf("Wrong max time")
		}
	})

	t.Run("TestAdvanceClusterTime", func(t *testing.T) {
		id, _ := uuid.New()
		sess, err := NewClientSession(&Pool{}, id, Explicit, OptCausalConsistency(true))
		require.Nil(t, err, "Unexpected error")
		err = sess.AdvanceClusterTime(clusterTime2)
		require.Nil(t, err, "Unexpected error")
		if sess.ClusterTime != clusterTime2 {
			t.Errorf("Session cluster time incorrect, expected %v, received %v", clusterTime2, sess.ClusterTime)
		}
		err = sess.AdvanceClusterTime(clusterTime3)
		require.Nil(t, err, "Unexpected error")
		if sess.ClusterTime != clusterTime2 {
			t.Errorf("Session cluster time incorrect, expected %v, received %v", clusterTime2, sess.ClusterTime)
		}
		err = sess.AdvanceClusterTime(clusterTime1)
		require.Nil(t, err, "Unexpected error")
		if sess.ClusterTime != clusterTime1 {
			t.Errorf("Session cluster time incorrect, expected %v, received %v", clusterTime1, sess.ClusterTime)
		}
		sess.EndSession()
	})

	t.Run("TestEndSession", func(t *testing.T) {
		id, _ := uuid.New()
		sess, err := NewClientSession(&Pool{}, id, Explicit, OptCausalConsistency(true))
		require.Nil(t, err, "Unexpected error")
		sess.EndSession()
		err = sess.UpdateUseTime()
		require.NotNil(t, err, "Expected error, received nil")
	})

	t.Run("TestAdvanceOperationTime", func(t *testing.T) {
		id, _ := uuid.New()
		sess, err := NewClientSession(&Pool{}, id, Explicit, OptCausalConsistency(true))
		require.Nil(t, err, "Unexpected error")

		optime1 := &bson.Timestamp{
			T: 1,
			I: 0,
		}
		err = sess.AdvanceOperationTime(optime1)
		testhelpers.RequireNil(t, err, "error updating first operation time: %s", err)
		compareOperationTimes(t, optime1, sess.OperationTime)

		optime2 := &bson.Timestamp{
			T: 2,
			I: 0,
		}
		err = sess.AdvanceOperationTime(optime2)
		testhelpers.RequireNil(t, err, "error updating second operation time: %s", err)
		compareOperationTimes(t, optime2, sess.OperationTime)

		optime3 := &bson.Timestamp{
			T: 2,
			I: 1,
		}
		err = sess.AdvanceOperationTime(optime3)
		testhelpers.RequireNil(t, err, "error updating third operation time: %s", err)
		compareOperationTimes(t, optime3, sess.OperationTime)

		err = sess.AdvanceOperationTime(&bson.Timestamp{
			T: 1,
			I: 10,
		})
		testhelpers.RequireNil(t, err, "error updating fourth operation time: %s", err)
		compareOperationTimes(t, optime3, sess.OperationTime)
		sess.EndSession()
	})

	t.Run("TestTransactionState", func(t *testing.T) {
		id, _ := uuid.New()
		sess, err := NewClientSession(&Pool{}, id, Explicit)
		require.Nil(t, err, "Unexpected error")

		err = sess.CommitTransaction()
		if err != ErrNoTransactStarted {
			t.Errorf("expected error, got %v", err)
		}

		err = sess.AbortTransaction()
		if err != ErrNoTransactStarted {
			t.Errorf("expected error, got %v", err)
		}

		if sess.state != None {
			t.Errorf("incorrect session state, expected None, received %v", sess.state)
		}

		err = sess.StartTransaction()
		require.Nil(t, err, "error starting transaction: %s", err)
		if sess.state != Starting {
			t.Errorf("incorrect session state, expected Starting, received %v", sess.state)
		}

		err = sess.StartTransaction()
		if err != ErrTransactInProgress {
			t.Errorf("expected error, got %v", err)
		}

		sess.ApplyCommand()
		if sess.state != InProgress {
			t.Errorf("incorrect session state, expected InProgress, received %v", sess.state)
		}

		err = sess.StartTransaction()
		if err != ErrTransactInProgress {
			t.Errorf("expected error, got %v", err)
		}

		err = sess.CommitTransaction()
		require.Nil(t, err, "error committing transaction: %s", err)
		if sess.state != Committed {
			t.Errorf("incorrect session state, expected Committed, received %v", sess.state)
		}

		err = sess.AbortTransaction()
		if err != ErrAbortAfterCommit {
			t.Errorf("expected error, got %v", err)
		}

		err = sess.StartTransaction()
		require.Nil(t, err, "error starting transaction: %s", err)
		if sess.state != Starting {
			t.Errorf("incorrect session state, expected Starting, received %v", sess.state)
		}

		err = sess.AbortTransaction()
		require.Nil(t, err, "error aborting transaction: %s", err)
		if sess.state != Aborted {
			t.Errorf("incorrect session state, expected Aborted, received %v", sess.state)
		}

		err = sess.AbortTransaction()
		if err != ErrAbortTwice {
			t.Errorf("expected error, got %v", err)
		}

		err = sess.CommitTransaction()
		if err != ErrCommitAfterAbort {
			t.Errorf("expected error, got %v", err)
		}
	})
}
