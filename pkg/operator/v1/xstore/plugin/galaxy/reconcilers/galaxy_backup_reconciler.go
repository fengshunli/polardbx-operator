/*
Copyright 2021 Alibaba Group Holding Limited.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconcilers

import (
	xstorev1 "github.com/alibaba/polardbx-operator/api/v1"
	"github.com/alibaba/polardbx-operator/pkg/k8s/control"
	xstorev1reconcile "github.com/alibaba/polardbx-operator/pkg/operator/v1/xstore/reconcile"
	backupsteps "github.com/alibaba/polardbx-operator/pkg/operator/v1/xstore/steps/backup"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type GalaxyBackupReconciler struct {
}

func (r *GalaxyBackupReconciler) Reconcile(rc *xstorev1reconcile.BackupContext, log logr.Logger, request reconcile.Request) (reconcile.Result, error) {
	backup := rc.MustGetXStoreBackup()
	log = log.WithValues("phase", backup.Status.Phase)

	task, err := r.newReconcileTask(rc, backup, log)
	if err != nil {
		log.Error(err, "Failed to build reconcile task.")
		return reconcile.Result{}, err
	}
	return control.NewExecutor(log).Execute(rc, task)
}

func (r *GalaxyBackupReconciler) newReconcileTask(rc *xstorev1reconcile.BackupContext, xstoreBackup *xstorev1.XStoreBackup, log logr.Logger) (*control.Task, error) {

	task := control.NewTask()

	defer backupsteps.PersistentStatusChanges(task, true)

	switch xstoreBackup.Status.Phase {
	case xstorev1.XStoreBackupNew:
		backupsteps.UpdateBackupStartInfo(task)
		backupsteps.CreateBackupConfigMap(task)
		backupsteps.StartXStoreFullBackupJob(task)
		backupsteps.UpdatePhaseTemplate(xstorev1.XStoreFullBackuping)(task)
	case xstorev1.XStoreFullBackuping:
		backupsteps.WaitFullBackupJobFinished(task)
		backupsteps.UpdatePhaseTemplate(xstorev1.XStoreBackupCollecting)(task)
	case xstorev1.XStoreBackupCollecting:
		backupsteps.WaitBinlogOffsetCollected(task)
		backupsteps.StartCollectBinlogJob(task)
		backupsteps.WaitCollectBinlogJobFinished(task)
		backupsteps.UpdatePhaseTemplate(xstorev1.XStoreBinlogBackuping)(task)
	case xstorev1.XStoreBinlogBackuping:
		backupsteps.WaitPXCSeekCpJobFinished(task)
		backupsteps.StartBinlogBackupJob(task)
		backupsteps.WaitBinlogBackupJobFinished(task)
		backupsteps.ExtractLastEventTimestamp(task)
		backupsteps.UpdatePhaseTemplate(xstorev1.XStoreBinlogWaiting)(task)
	case xstorev1.XStoreBinlogWaiting:
		backupsteps.WaitPXCBackupFinished(task)
		backupsteps.SaveXStoreSecrets(task)
		backupsteps.UpdatePhaseTemplate(xstorev1.XStoreBackupFinished)(task)
	case xstorev1.XStoreBackupFinished:
		backupsteps.RemoveFullBackupJob(task)
		backupsteps.RemoveCollectBinlogJob(task)
		backupsteps.RemoveBinlogBackupJob(task)
		backupsteps.RemoveXSBackupOverRetention(task)
		log.Info("Finished phase.")
	default:
		log.Info("Unrecognized phase.")
	}

	return task, nil
}
